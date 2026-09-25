# FlatNas · 个人定制版

> 基于 [Garry-QD/FlatNas](https://github.com/Garry-QD/FlatNas)（上游 1.2.6）的个人定制分支。
> 技术栈：**Vue 3 + TypeScript + Vite + Pinia**（前端） / **Go + Gin + socket.io**（后端）。

FlatNas 是一个轻量级、高度可定制的**个人导航页与仪表盘**：把常用网站、内网服务、书签、待办、RSS、媒体与各类小组件聚合到一个浏览器起始页，数据全部存储在自己的服务器上。

- 🌐 多端统一入口：一套配置，桌面 / 移动端自适应。
- 🔒 数据本地可控：配置以 JSON 落盘，迁移与备份就是拷一个目录。
- 🧩 组件化布局：自由拖拽网格，支持多种尺寸与自定义 CSS/JS。
- ⚡ 轻量：NAS 端常驻内存约 100MB，访问端真实内存占用 < 80MB。

![debian/1.png](public/1.png)
![debian/7.png](public/7.png)
![debian/9.png](public/9.png)

---

## ✨ 功能概览

| 领域 | 能力 |
| --- | --- |
| 导航 | 网格布局、分组管理、书签卡片（支持内网/外网智能切换）、图标库 |
| 小组件 | 时钟 / 天气 / 日历 / 待办 / 备忘录 / 计算器 / 倒计时 / 计数 / IP / 热搜 / RSS / 音乐 / 文件传输 / iframe / 高德地图天气 / 自定义 CSS / 状态监控 等 |
| 媒体 | 壁纸管理（PC / 移动端分开）、本地音乐播放器（MiniPlayer）、文件传输助手（文本/文件/图片，断点续传） |
| 定制 | 自定义 CSS / JS / HTML、卡片与分组样式、图标底色（Hex） |
| 网络 | 内外网自动识别与路由、后端代理转发（`PROXY_URL`） |
| 多用户 | 单用户 / 多用户模式、共享分组、访问码保护、配置自动保存 |

### 🎯 本分支相对上游的定制

> 本仓库是**去广告、去无关功能、并增强多用户与多端可靠性**的定制版，与上游存在明显差异，请勿混用镜像与文档。

**新增**

- **共享分组**：管理员可把自己维护的某个分组标记为「共享」，所有已登录用户都会看到该分组的只读副本，复用同一批书签而无需各自维护。
- **分组混排偏好**：非管理员可自由调整「自己的分组 + 只读共享分组」的展示顺序，偏好保存在各自的数据文件中。
- **访问码保护（隐藏分组）**：可为分组设置隐藏标记，未解锁时**服务端直接不下发**这些分组（而非前端隐藏）。入口隐蔽（连续点击站点标题 3 次触发）。解锁有效期可配置（会话内 / 指定小时数），访问码由管理员统一设置。
- **配置自动保存**：分组、顺序等改动会按可配置的防抖间隔自动落盘，页面关闭时另有兜底写入；设置页可调整延迟或关闭。
- **多端配置保存（字段级三方合并）**：多设备/多标签页同时编辑时，后端按 `base / local / server` **逐字段合并**，不同字段的各改各的互不覆盖；同一字段冲突时以服务端为准并回报，避免「整份文档互相覆盖」。

**移除**

- ❌ **Docker 管理组件**（容器查看/启停/升级镜像等）及其后端接口。
- ❌ **宿主机系统状态组件**（CPU / 内存 / 磁盘 / 网络监控）及其后端接口。
- ❌ 上游作者的相关引流信息。

> 由于已移除 Docker 管理，部署时**不再需要挂载 `/var/run/docker.sock`**。

---

## 🖥️ 仪表盘与布局

- **网格布局**：自由拖拽，组件支持多种尺寸。
- **分组管理**：多分组分类管理应用与书签，支持分组图标与统一卡片样式。
- **响应式设计**：桌面端与移动端分别适配（独立壁纸与遮罩配置）。
- **编辑模式**：所见即所得，添加 / 删除 / 排列组件与分组。
- **亮暗色模式** 与 **日间/夜间遮罩**自动切换。

## 🧩 小组件

![tools.png](public/tools.png)

- **文件传输助手**：跨设备收发文本、文件与图片，支持大文件与断点续传，提供图片归类预览视图。
- **书签组件**：快速访问常用网站，支持自定义图标；首次启动自动填充常用站点。
- **时钟 / 天气**：实时时间、日期与当地天气（支持高德天气；可显示天气动效）。
- **日历**：支持农历。
- **待办 / 备忘录**：轻量任务与随记，内容独立落盘。
- **RSS 订阅**：内置 RSS 阅读器。
- **热搜榜单**：聚合微博热搜等即时热点。
- **计算器 / 倒计时 / 计数 / IP 展示**：常用小工具集合。
- **音乐播放器**：播放服务器本地音乐目录中的音频。
- **iframe / 自定义 HTML / 自定义 CSS 组件**：嵌入外部页面或注入自定义内容。
- **高德地图**：显示当前位置与地图导航。
- **状态监控**：前端运行时的页面/网络状态指示（与已移除的宿主机监控无关）。

![高德天气.png](public/高德天气.png)

## 🎨 个性化定制

内置图标库（含在线图标匹配），满足不同风格需求：

![icon.png](public/icon.png)

- **自定义组件 / CSS / JS / HTML**：深度扩展页面行为与样式。
- **图标管理**：内置图标库，支持上传自定义图片，全面支持 **Hex 颜色**（如 `#ffffff`）自定义图标底色。
- **背景设置**：PC 与移动端壁纸分别配置，支持模糊 / 遮罩强度、轮播与昼夜遮罩。
- **分组卡片背景**：在分组设置中统一配置该组所有卡片背景（图片 / 模糊 / 遮罩）。
- **访客统计**：页脚显示总访问量、今日访问量及在线时长（需在设置中开启）。
- **更新提醒**：自动检测 GitHub 最新 Release。

![自定义脚本.png](public/自定义脚本.png)

---

## 👥 多用户与访问控制

- **认证模式**：`single`（单用户）或 `multi`（多用户）。
  - 单用户：所有配置保存在 `server/data/data.json`。
  - 多用户：每个用户在 `server/data/users/<用户名>.json`，会话基于 JWT。
- **默认密码**：`admin`，请登录后立即在设置中修改。
- **共享分组**：管理员在分组设置中开启「共享给所有用户」。
- **访问码保护**：管理员在设置中设置全局访问码；成员对指定分组开启隐藏标记后，未解锁时服务端不下发这些分组。连点站点标题 3 次呼出解锁入口，可为解锁设置有效期。

---

## ⚙️ 配置说明

| 项 | 说明 |
| --- | --- |
| 数据文件 | 单用户 `server/data/data.json`；多用户 `server/data/users/<用户名>.json` |
| 系统配置 | `server/data/system.json`（认证模式、全局访问码等） |
| 音乐目录 | 将音频放入 `server/music`，刷新后即可在播放器中看到 |
| 壁纸目录 | PC 壁纸 `server/PC`、移动端壁纸 `server/APP` |
| 文件传输目录 | `server/doc` |

**常用环境变量**

| 变量 | 作用 | 默认 |
| --- | --- | --- |
| `HOST` | 监听地址 | `0.0.0.0` |
| `PORT` | 监听端口 | `3000` |
| `BASE_DIR` | 数据根目录 | `/app` |
| `BASE_PATH` | 子路径部署前缀，如 `/flatnas` | 空 |
| `PROXY_URL` | 后端代理地址，支持 `http/https/socks5/socks5h` | 空（禁用） |
| `CORS_ALLOW_ORIGINS` | 允许的跨域来源 | 空 |

---

## 📦 安装与部署

### 1. Docker Compose（推荐）

```yaml
version: "3.8"

services:
  flatnas:
    image: qdnas/flatnas:latest
    container_name: flatnas
    restart: unless-stopped
    ports:
      - "23000:3000"
    environment:
      - HOST=0.0.0.0
      - PORT=3000
      # - BASE_PATH=/flatnas        # 子路径部署时启用
      # - PROXY_URL=http://127.0.0.1:7890
    volumes:
      - ./data:/app/server/data
      - ./doc:/app/server/doc
      - ./music:/app/server/music
      - ./PC:/app/server/PC
      - ./APP:/app/server/APP
```

> 仓库内 `docker-compose.yml` 为上游模板，可能仍含 `docker.sock` 挂载项 —— 本定制版已移除 Docker 管理，可自行删除该挂载。

### 2. Docker CLI

```bash
docker run -d \
  -p 23000:3000 \
  -v $(pwd)/data:/app/server/data \
  -v $(pwd)/doc:/app/server/doc \
  -v $(pwd)/music:/app/server/music \
  -v $(pwd)/PC:/app/server/PC \
  -v $(pwd)/APP:/app/server/APP \
  --name flatnas \
  qdnas/flatnas:latest
```

### 3. 本地二进制（Release 包）

```bash
# 下载对应架构的 flatnas-amd64.zip / flatnas-arm64.zip 并解压到任意目录
cd /opt/flatnas
chmod +x flatnas-server
./flatnas-server
```

访问 `http://<服务器IP>:3000`。

### 4. Debian / Ubuntu 脚本部署

```bash
wget -O deploy_debian.sh https://raw.githubusercontent.com/utterliar1/FlatNas/main/deploy_debian.sh
chmod +x deploy_debian.sh && sudo ./deploy_debian.sh
```

配套管理脚本 `manage.sh` 提供状态查看、端口修改、HTTPS 配置、日志与卸载等操作：

```bash
wget -O manage.sh https://raw.githubusercontent.com/utterliar1/FlatNas/main/manage.sh
chmod +x manage.sh && sudo ./manage.sh
```

### 5. 从源码构建

```bash
# 后端（Go 1.25+）
cd backend
go build -ldflags="-s -w" -o flatnas-backend .

# 前端（Node 20+）
cd ../frontend
npm install
npm run build-only     # 产物输出到 ../server/public
```

---

## 🌐 代理配置

当内网服务无法直连外网，或需要隐藏真实来源时，可让后端代为转发。

1. 设置环境变量 `PROXY_URL`（支持 `http://`、`https://`、`socks5://`、`socks5h://`，格式 `protocol://[user:pass@]host:port`）。
2. 前端在卡片/组件配置中出现「代理」开关，开启后该组件的请求经后端转发。

**排查**

- 开关不显示：检查后端日志确认 `PROXY_URL` 已生效；`GET /api/config/proxy-status` 可查看代理可用状态。
- 请求失败：确认代理可达；注意 SSRF 防护（如禁止访问 `localhost`）；查看后端日志 `[Proxy Error]`。

## 🌐 智能网络环境检测

后端结合**客户端 IP**、**访问域名**与**网络延迟**三个维度判断用户处于内网还是公网，并自动路由到配置的内网 / 外网地址，实现「同一个图标，内外网各自最优」的无感切换。

---

## 🎨 全局自定义 CSS

在 **设置 → 自定义 CSS** 中编写全局样式。支持如下标签，自动转换为媒体查询：

- `<mobile>…</mobile>`：仅移动端（`max-width: 768px`）
- `<desktop>…</desktop>`：仅桌面端（`min-width: 769px`）
- `<dark>…</dark>` / `<light>…</light>`：仅暗色 / 亮色模式

```css
::-webkit-scrollbar { width: 6px; }

<mobile>
.sidebar { display: none; }
</mobile>
```

## ⚡ 全局自定义 JS

在 **设置 → 自定义 JS** 中注入脚本（首次启用需同意安全免责声明）。代码运行在沙箱中，并注入 `ctx` 上下文对象；推荐使用生命周期钩子：

```javascript
// @module
export default {
  init(ctx) {
    ctx.on("widget-click", (e) => console.log("Widget clicked:", e.detail));
  },
  update(ctx) { /* 配置变更时调用 */ },
  destroy(ctx) { /* 清理定时器 / 事件监听 */ },
};
```

---

## 📁 目录结构

```
FlatNas/
├── backend/            # Go(Gin) 后端
│   └── handlers/       # 路由处理器（数据、鉴权、共享分组、访问码、合并等）
├── frontend/           # Vue 3 + TS 前端
│   └── src/{components,stores,composables,utils,locales}
├── server/             # 运行期目录
│   ├── public/         # 前端构建产物
│   ├── data/           # 数据（data.json / users / system.json）
│   ├── doc music PC APP
├── docker-compose.yml
├── deploy_debian.sh / manage.sh
└── README.md
```

## 📜 开源协议

本项目采用 [GNU AGPLv3](LICENSE) 开源，遵循上游授权。上游项目：[Garry-QD/FlatNas](https://github.com/Garry-QD/FlatNas)。

---

Enjoy your FlatNas! 🚀
