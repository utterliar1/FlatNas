# FlatNas 前端工程 (个人定制版)

[![GitHub](https://img.shields.io/badge/GitHub-FlatNas-181717?style=flat&logo=github&logoColor=white)](https://github.com/utterliar1/FlatNas)
[![Vue 3](https://img.shields.io/badge/Vue-3.x-42b883?style=flat&logo=vue.js&logoColor=white)](https://vuejs.org/)
[![Vite](https://img.shields.io/badge/Vite-5.x-646CFF?style=flat&logo=vite&logoColor=white)](https://vitejs.dev/)

FlatNas 前端界面基于 Vue 3、Vite 与 Tailwind CSS 构建，提供流畅的拖拽交互、自适应布局以及丰富的小组件生态。

> **📌 致谢与声明**：本项目基于 [FlatNas (Garry-QD/FlatNas)](https://github.com/Garry-QD/FlatNas) 二次开发，由衷感谢原作者 **Garry-QD** 及其社区贡献者的辛勤付出！本项目仅供个人日常自用与学习交流。

---

## 🛠️ 本地开发与调试

### 1. 安装依赖

```bash
cd frontend
npm install
```

### 2. 开发服务器启动

```bash
# 启动前端开发调试服务 (默认端口 5173)
npm run dev

# 编译生产环境静态资源 (输出至 frontend/dist)
npm run build
```

### 3. 与后端协同调试

确保本地已配置 Go 运行环境，并进入 `backend` 目录运行后端服务：

```bash
cd backend
go run .
```

---

## 📜 开源协议

遵循 [GNU AGPLv3](LICENSE) 开源协议。
