# FlatNas (个人定制版)

[![GitHub](https://img.shields.io/badge/GitHub-FlatNas-181717?style=flat&logo=github&logoColor=white)](https://github.com/utterliar1/FlatNas)
[![Vue 3](https://img.shields.io/badge/Frontend-Vue%203%20%2B%20Vite-42b883?style=flat&logo=vue.js&logoColor=white)](https://vuejs.org/)
[![Go Gin](https://img.shields.io/badge/Backend-Go%20%28Gin%29-00add8?style=flat&logo=go&logoColor=white)](https://gin-gonic.com/)
[![License](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)

FlatNas 是一个轻量级、高度可定制的个人 NAS 导航页与仪表盘系统。前端基于 Vue 3 + Vite 构建，后端采用高效轻量的 Go (Gin) 框架，致力于为私有云、NAS 及个人服务器打造清爽、高效、便捷的浏览器起始页。

> ### 📌 项目致谢与声明
> - **开源致谢**：本项目基于上游优秀的开源项目 [FlatNas (Garry-QD/FlatNas)](https://github.com/Garry-QD/FlatNas) 二次开发。在此由衷感谢原作者 **Garry-QD** 及其社区贡献者的辛勤付出与开源共享！
> - **自用声明**：**本项目仅为个人日常自用与学习定制版本**，结合个人家庭网络环境与日常使用习惯进行了精简与体验增强，不作任何商业用途。

![预览图](public/1.png)

---

## ✨ 核心特性与定制增强

### 1. 专注纯粹的轻量架构
- **聚焦仪表盘本质**：移除了内置 Docker 容器管理等外围运维逻辑，无需向容器挂载宿主机 `/var/run/docker.sock`，进一步收敛系统权限边界，降低潜在安全风险。
- **极致资源占用**：整体镜像采用 Alpine 静态编译打包，运行时内存占用仅几十兆，响应迅速敏捷。

### 2. 精准的智能网络判定与直达
- **多维度网络环境感知**：综合分析“客户端访问 IP / 访问域名 / 网络延时”三大维度，并在后台集成**内网探针目标（Lan Probe Target）**探测能力，准确识别当前设备所处网络状态（局域网直连或公网远程）。
- **游客免登录内网直达**：支持开启 `allowGuestLanAccess`，家庭成员或内网访客即便处于未登录状态，只要处于局域网环境即可直达内网服务，兼顾便利性与数据边界。

### 3. 便捷的人机交互与管理体验
- **聚合搜索引擎增强**：搜索栏支持点击引擎图标一键循环切换（Google、Bing、百度、DuckDuckGo、搜狗等），支持自定义搜索引擎图标与配色方案，随手即可高效检索。
- **纯净模式暗门机制**：日常使用时提供极简清爽的只读视界；通过连续三击右上角状态徽标即可解锁完整管理员编辑与全局设置入口，防止误触且页面更为整洁。
- **图标裁剪与交互修复**：优化图标与底图上传裁剪交互，全面支持裁剪框拖拽缩放。
- **自动防抖持久化**：小组件排序、侧边栏拖拽及配置调整自动触发防抖保存，告别繁琐的手动保存。

### 4. 丰富的内置组件生态
![组件展示](public/tools.png)

- **跨设备文件传输助手**：支持文本、大文件与图片互传，支持断点续传与图片分类预览。
- **实用小工具集**：内置待办清单 (Todo)、RSS 订阅阅读器、新闻热榜资讯、简易计算器、多风格时钟与天气预报。
- **高德地图与天气**：支持高德地图与本地实时天气小组件。
- **本地音乐播放器**：内置 MiniPlayer，支持读取并流式播放本地音频资源。
- **系统资源监控**：实时呈现宿主机 CPU、内存、磁盘与网络运行状态。
- **万能嵌入与扩展**：支持 iframe 外部网页嵌入，并提供沙箱级自定义 HTML / CSS / JS 脚本支持。

---

## 🖥️ 个性化定制

- **海量图标支持**：支持本地图标上传、外部 URL 图标及 Hex 颜色代码背景自定义。
- **壁纸与视觉风格**：支持自定义 PC/移动端壁纸、高斯模糊、遮罩透明度及深色/浅色模式无缝适配。
- **全局自定义样式与脚本**：支持在设置面板中编写专属 CSS（支持 `<mobile>`、`<desktop>` 自定义标签）与 JS 生命周期钩子，随心打造专属风格。

![自定义脚本](public/自定义脚本.png)

---

## 📦 部署指南 (推荐 Docker Compose)

本项目已剔除 `docker.sock` 挂载，仅需映射常规数据卷即可开箱即用。

### 1. 目录结构准备

建议在主机创建如下数据挂载目录：

```bash
mkdir -p flatnas/{data,doc,music,PC,APP}
cd flatnas
```

### 2. 编写 `docker-compose.yml`

```yaml
version: '3.8'

services:
  flatnas:
    image: flatnas:latest
    container_name: flatnas
    restart: unless-stopped
    ports:
      - '23000:3000'
    environment:
      - HOST=0.0.0.0
      - PORT=3000
      # 如需配置网络代理，可启用以下变量：
      # - PROXY_URL=http://127.0.0.1:7890
    volumes:
      - ./data:/app/server/data
      - ./doc:/app/server/doc
      - ./music:/app/server/music
      - ./PC:/app/server/PC
      - ./APP:/app/server/APP
```

### 3. 启动容器

```bash
docker compose up -d
```

启动完成后，通过浏览器访问 `http://<服务器IP>:23000` 即可开始使用。

---

## ⚙️ 配置说明

- **初始密码**：系统默认初始密码为 `admin`，首次登录后请在设置面板中及时修改。
- **配置持久化**：所有页面布局、组件参数及书签数据均存储于本地挂载卷 `data/data.json` 中，备份迁移极为方便。
- **音乐文件**：将 MP3 音频文件放置于挂载卷 `music/` 目录下，刷新页面即可在音乐播放组件中加载收听。

---

## 🌐 后端网络代理配置

若您的部分内网组件或外链资源需要通过统一代理访问：

1. 在 `docker-compose.yml` 中设置环境变量：
   ```yaml
   environment:
     - PROXY_URL=http://127.0.0.1:7890
     # 支持协议：http, https, socks5, socks5h
   ```
2. 重启容器后，在对应卡片或组件设置中即可勾选开启代理转发。

---

## 📜 开源协议与致谢

- 本项目代码遵循 [GNU AGPLv3](LICENSE) 协议开源。
- 再次感谢原项目 [FlatNas](https://github.com/Garry-QD/FlatNas) 原作者 **Garry-QD** 为开源社区带来的优秀项目！
