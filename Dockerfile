# FlatNas 镜像：三段式构建（前端 Vite / 后端 Go / 精简运行层）
#
# 基础镜像版本策略：固定到「次版本线」而不是 latest ——
#   既不会被 latest 悄悄带动（alpine:latest 目前是 3.24.2，下个大版本发布就会跳到 3.25），
#   又能自动拿到该版本线内的安全补丁。
#   · node:24-bookworm-slim —— Node 24 是当前 Active LTS（EOL 2028-04）。
#     原先是 node:20.19，Node 20 已于 2026-04-30 EOL，不再有安全更新。
#   · golang:1.27-alpine    —— 与 backend/go.mod 的工具链代际一致（本地/CI 均为 1.27.1）。
#   · alpine:3.24           —— 运行层，当前 stable。
#
# 两个构建阶段都显式声明 --platform=$BUILDPLATFORM：
#   两阶段的产物本身与目标架构无关（前端是 JS/CSS 包，后端用 GOOS/GOARCH 交叉编译），
#   让它们跑在构建机原生架构上、而不是 QEMU 模拟里；只有运行层需要按目标架构真正构建。
#   这既让多架构构建不至于慢几倍，也避免了跨架构下 esbuild/rollup 原生依赖取错平台包。

# ---------- Stage 1: 构建前端 ----------
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS frontend-builder

# 代理与镜像源：网络受限环境（如国内直连 npm 慢）可在构建时传入
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NPM_REGISTRY=https://registry.npmjs.org

ENV HTTP_PROXY=$HTTP_PROXY \
    HTTPS_PROXY=$HTTPS_PROXY \
    NPM_CONFIG_REGISTRY=$NPM_REGISTRY

WORKDIR /app/frontend

# 先只拷依赖清单：只要 package-lock.json 没变，下面这层 npm ci 就一直命中缓存。
# （注意不要把源码或构建产物拷到这一层之前 —— 那会让 npm ci 每次重跑。）
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --prefer-offline --no-audit --timeout=300000

# 源码 + 静态素材（frontend/public 是 publicDir，会整体进 dist）
COPY frontend/ .

# VITE_DOCKER_BUILD=1 → vite.config 的 outDir 取 "dist"（而非 Windows 下的 ../server/public）
ENV TAILWIND_DISABLE_NATIVE=1 \
    VITE_DOCKER_BUILD=1
RUN npm run build-only

# ---------- Stage 2: 构建后端 ----------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS backend-builder

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG GOPROXY=https://proxy.golang.org,direct

ENV HTTP_PROXY=$HTTP_PROXY \
    HTTPS_PROXY=$HTTPS_PROXY \
    GOPROXY=$GOPROXY

WORKDIR /app/backend

# 同上：依赖清单单独一层，源码变动不触发重新下载模块
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .

# 交叉编译到目标架构的静态二进制
# -trimpath 去掉构建机绝对路径，产物可复现
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o flatnas-backend .

# ---------- Stage 3: 运行层 ----------
FROM alpine:3.24

# ca-certificates：出站 HTTPS（天气 / RSS / 图标抓取）
# tzdata：正确时区；mkdir：为数据卷准备挂载点（后端 config.ensureDirs 也会兜底创建，
#         这里先建好，挂载空目录时行为与未挂载时一致）
# 说明：不切非 root 用户 —— 现有部署的数据卷都是 root 属主，改 USER 会导致无法写入。
RUN apk --no-cache add ca-certificates tzdata && \
    mkdir -p /app/server/data \
             /app/server/doc \
             /app/server/music \
             /app/server/APP \
             /app/server/PC

ENV TZ=Asia/Shanghai \
    GIN_MODE=release \
    BASE_DIR=/app

WORKDIR /app

COPY --from=backend-builder /app/backend/flatnas-backend .
# 前端产物整体落到运行期的静态目录（后端 config.PublicDir）
COPY --from=frontend-builder /app/frontend/dist ./server/public

EXPOSE 3000

# 健康检查：/api/system-config 是公开端点且只读缓存配置，开销极低。
# HOST/PORT/BASE_PATH 都可由环境变量覆盖（见 backend/main.go），这里跟着走，
# 避免用户改了端口反而把容器判成 unhealthy。
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -q --spider "http://${HOST:-127.0.0.1}:${PORT:-3000}${BASE_PATH%/}/api/system-config" || exit 1

CMD ["./flatnas-backend"]
