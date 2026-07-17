# TimeNotes Blog

TimeNotes 手账本的公开展示与协作上传服务。

- **前端**：React + TypeScript + Semi Design（毛玻璃 / 动态光效）
- **后端**：Go + Fiber v3 + WebSocket + SQLite
- **部署形态**：前端 `npm run build` 产物输出到 `web/`，`go build` 时通过 `//go:embed` 打进可执行文件，运行时同端口托管（无需再拷贝 `web/` 目录）
- **业务 API**：几乎全部走 WebSocket；HTTP 仅用于静态页、健康检查、`.tnote` 下载

<p align="center">
  <img alt="Version" src="https://img.shields.io/badge/version-2.9.0-blue?style=flat-square" />
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square" />
  <img alt="Platform" src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20amd64-5B8DEF?style=flat-square" />
</p>

## 💝 支持项目

<p align="center">
  TimeNotes 是独立开发者用爱发电的开源项目，完全免费。<br/>
  如果它帮你写出了更棒的手账，欢迎请开发者喝杯咖啡 ☕
</p>

<p align="center">
  <a href="https://ifdian.net/a/algfwq">
    <img src="./afdian-sponsor.jpg" alt="爱发电赞助 TimeNotes" width="480" />
  </a>
</p>

<p align="center">
  👉 <a href="https://ifdian.net/a/algfwq"><strong>前往爱发电支持 TimeNotes</strong></a>
</p>

## 版本 2.9.0 更新摘要

- 生产构建产物统一命名：`TimeNotesBlog-2.9.0-{arch}-{os}`（`//go:embed` 内嵌当前 `web/` 前端构建；单独拷贝 exe 即可部署）。
- 与客户端 2.9.0 对齐：桌面 / Android 均可「连接到 Blog」上传或更新完整 `.tnote`。
- 客户端 Android 侧通过原生 Go 代理发起 Blog WebSocket，避免 `https://wails.localhost` 混合内容拦截。
- 账号体系：管理员后台发号、argon2id 密码、PoW 登录、JWT 会话；后台路径 token 每次启动随机。
- 互动：按 IP 点赞、评论、访问统计与可插拔 GeoIP。

## 功能概览

| 模块 | 说明 |
|------|------|
| 公开首页 | 展示所有可见手账，标注上传者 |
| 阅读页 | 浏览器解压完整 `.tnote` 并只读渲染；点赞（按 IP，不可取消）；评论侧栏 |
| 客户端上传 | TimeNotes 客户端连接 Blog 后上传 / 更新完整 `.tnote` |
| 账号 | 不开放注册；管理员后台添加用户；登录 API + PoW + JWT |
| 后台 | `/admin/{每次启动随机 token}/`；用户/手账/统计（VChart） |
| 编辑手账 | 后台调用本机客户端 `127.0.0.1:54088` 桥下载并打开 |

---

## 发布产物（v2.9.0）

位于 `bin/`，命名规则：`TimeNotesBlog-{version}-{arch}-{os}`。

| 文件 | 说明 |
|------|------|
| `bin/TimeNotesBlog-2.9.0-amd64-windows.exe` | Windows amd64 生产可执行文件（**已内嵌**前端静态资源） |
| `bin/TimeNotesBlog-2.9.0-amd64-linux` | Linux amd64 生产可执行文件（**已内嵌**前端静态资源） |

**前端已在编译时通过 `//go:embed` 打进二进制**，部署时**不必**再拷贝 `web/` 目录，也**不必**在目标机器安装 Go / Node.js。

---

## 必做：使用预构建产物部署（推荐）

适合直接拿 `bin/` 里的可执行文件上线，无需从源码编译。

### 1. 需要准备的文件

在目标机器上准备一个**工作目录**（例如 `D:\blog` 或 `/opt/timenotes-blog`），放入：

| 文件 | 是否必须 | 说明 |
|------|----------|------|
| `TimeNotesBlog-2.9.0-amd64-windows.exe` 或 `TimeNotesBlog-2.9.0-amd64-linux` | **必须** | 预构建可执行文件 |
| `config.json` | **必须** | 由仓库中的 `config.example.json` 复制并修改 |

**不需要**随包拷贝：

- `web/`（前端已内嵌；浏览器访问 `/` 即由二进制提供页面与 JS/CSS）
- `frontend/`、Go 源码、Node.js 依赖

首次运行后，程序会在工作目录下**自动创建**（路径以配置为准）：

```
<data 目录>/          # 默认 data/blog.db、data/notes/、data/covers/、data/site/
logs/                 # 默认 logs/timenotes-blog.log
```

### 2. 配置

在可执行文件所在目录：

```bash
# Linux / macOS
cp config.example.json config.json

# Windows PowerShell
Copy-Item config.example.json config.json
```

**生产必改：**

```json
{
  "addr": "0.0.0.0:8090",
  "jwtSecret": "请换成足够长的随机字符串（建议 ≥32 字符）"
}
```

| 配置项 | 建议 |
|--------|------|
| `addr` | 本机调试可用 `127.0.0.1:8090`；对外服务用 `0.0.0.0:8090` |
| `jwtSecret` | **生产必须**设置强随机串；过弱会导致进程拒绝启动 |
| `dbPath` / `notesDir` / `logPath` | 默认为相对路径，相对**进程工作目录**；生产可改为绝对路径 |

也可用环境变量覆盖（优先级高于配置文件部分字段）：

| 变量 | 含义 |
|------|------|
| `TIMENOTES_BLOG_CONFIG` | 配置文件路径 |
| `TIMENOTES_BLOG_ADDR` | 监听地址 |
| `TIMENOTES_BLOG_JWT_SECRET` | JWT 密钥 |
| `TIMENOTES_BLOG_DB` | SQLite 路径 |
| `TIMENOTES_BLOG_NOTES_DIR` | `.tnote` 存储目录 |
| `TIMENOTES_BLOG_LOG` | 日志路径 |
| `TIMENOTES_BLOG_CORS_ORIGINS` | 逗号分隔 Origin 白名单 |
| `TIMENOTES_BLOG_GEO_URL` | 自定义 GeoIP URL 模板（含 `{ip}`） |
| `TIMENOTES_BLOG_GEO_API_KEY` | GeoIP API Key（若需要） |

### 3. 启动

**Windows（PowerShell）：**

```powershell
cd D:\path\to\blog   # 与 exe、config.json 同一目录
.\TimeNotesBlog-2.9.0-amd64-windows.exe
```

**Linux：**

```bash
cd /opt/timenotes-blog   # 与二进制、config.json 同一目录
chmod +x TimeNotesBlog-2.9.0-amd64-linux
./TimeNotesBlog-2.9.0-amd64-linux
```

成功时启动日志会出现类似：

```
Serving frontend from embed:web/
Admin UI: http://127.0.0.1:8090/admin/<随机token>/
Default admin account created: username=admin password=123456
```

含义：

| 日志 | 含义 |
|------|------|
| `Serving frontend from embed:web/` | 正在使用二进制内嵌前端（正常生产路径） |
| `Serving frontend from disk:web/` | 工作目录下存在完整 `web/assets/`，优先用磁盘前端（开发迭代时常见，预构建部署一般不会出现） |
| `Admin UI: ...` | **每次重启 token 都会变**，请从本次启动的 stdout 复制完整 URL |
| 默认管理员 | 仅**第一次**建库时创建：`admin` / `123456`，登录后台后立刻改密 |

浏览器访问 `http://<主机>:<端口>/` 应看到 Blog 首页，而不是「请构建前端」提示页。

**工作目录注意：**

- 相对路径的 `config.json`、`data/`、`logs/` 都相对**进程当前工作目录**，不是相对 exe 所在路径。
- 请在「放有 `config.json` 的目录」里启动；用 systemd / 计划任务时请显式设置 `WorkingDirectory`。
- 也可用 `TIMENOTES_BLOG_CONFIG=/绝对路径/config.json` 指定配置，并把配置内的 `dbPath`、`notesDir`、`logPath` 写成绝对路径。

### 4. Linux 防火墙（若需外网访问）

```bash
# ufw 示例
sudo ufw allow 8090/tcp
sudo ufw reload
```

若前面有反向代理，需放行代理到本机的端口，并开启 WebSocket 升级。

### 5. 客户端连接

1. 启动 TimeNotes 客户端（会在 `127.0.0.1:54088` 启动本地桥）
2. 首页点击「连接到 Blog」
3. 填写 Blog URL / 用户名 / 密码，可勾选「记住密码」
4. 连接成功后，在手账卡片菜单「上传到 Blog」/「更新到 Blog」
5. 上云手账会显示 Semi 云图标标签；本地删除不影响服务器副本

---

## 从源码构建发布产物（开发者）

仅在需要自己编译、或改代码后重新打包时使用。目标机器若只用预构建二进制，可跳过本节。

### 环境

- Go 1.26+
- Node.js 18+ / npm

### 步骤

```powershell
# 1) 先构建前端 → 输出到 ../web/
cd TimeNotesBlog/frontend
npm install
npm run build
cd ..

# 2) 再打包二进制（会把当前 web/ 通过 go:embed 打进 exe）
# Windows amd64
$env:CGO_ENABLED=0; $env:GOOS="windows"; $env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o bin/TimeNotesBlog-2.9.0-amd64-windows.exe .

# Linux amd64
$env:CGO_ENABLED=0; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o bin/TimeNotesBlog-2.9.0-amd64-linux .
```

**顺序必须是：先 `npm run build`，再 `go build`。** 若 `web/` 缺少 `index.html` 或 `assets/`，编译会失败或产物无法正确提供页面。

本地开发也可：

```bash
cd TimeNotesBlog
# 终端 1：改前端时
cd frontend && npm run build   # 或使用 vite dev（仅前端热更时）

# 终端 2：跑后端（会优先使用磁盘 web/，无需每次重新 go build）
go run .
```

---

## 选做

| 项 | 说明 |
|----|------|
| HTTPS / 反代 | Nginx/Caddy 终止 TLS，并代理 `/` 与 `/ws` |
| systemd | 写 unit 守护进程（非必须）；务必设置 `WorkingDirectory` |
| Docker | 自行打包（非必须） |
| 高精度 GeoIP | 配置 `geo.provider=http_json` + 带 Key 的 URL |
| PostgreSQL | 业务依赖 `storage.Store` 接口，可后续新增实现 |
| CORS 精确白名单 | 公网部署时设置 `corsOrigins`，并视情况关闭 `allowLoopbackOrigins` |

### Nginx WebSocket 示例（选做）

```nginx
location / {
  proxy_pass http://127.0.0.1:8090;
  proxy_http_version 1.1;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
}
```

若使用反代，请把代理 IP 写入 `trustedProxies`，以便正确识别访客 IP（点赞 / 限流 / Geo）。

### systemd 示例（选做）

```ini
[Unit]
Description=TimeNotes Blog
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/timenotes-blog
ExecStart=/opt/timenotes-blog/TimeNotesBlog-2.9.0-amd64-linux
Restart=on-failure
# 可选：Environment=TIMENOTES_BLOG_CONFIG=/opt/timenotes-blog/config.json

[Install]
WantedBy=multi-user.target
```

---

## 安全说明

1. **默认管理员** `admin` / `123456` 仅首次初始化；务必修改。
2. **后台路径 token 每次重启都会变**，请从启动日志复制。
3. 密码使用 **argon2id** 哈希存储。
4. 登录需要 **PoW**；同一 IP 失败次数越多难度越高。上传等已登录操作使用 **JWT**，不再做 PoW。
5. 点赞按 `sha256(pepper|ip)` 去重，不可取消。
6. 评论支持「昵称+邮箱」或 GitHub 主页链接；GitHub 头像仅解析 `github.com/{user}`，不服务端抓任意 URL。
7. 上传文件名经路径净化；磁盘以 UUID 文件名存储。
8. 日志不记录密码、JWT、PoW 答案；Geo 失败时日志只写 IP 哈希前缀。
9. 生产必须设置强 `jwtSecret`。

---

## GeoIP（可切换多源）

默认：

```json
"geo": {
  "provider": "ip-api",
  "urlTemplate": "http://ip-api.com/json/{ip}?fields=status,message,country,regionName,city,lat,lon,query",
  "timeoutMs": 3000,
  "cacheTTLHours": 168
}
```

换成其它 HTTP JSON 源时：

```json
"geo": {
  "provider": "http_json",
  "urlTemplate": "https://example.com/json/{ip}?key={apiKey}",
  "apiKey": "YOUR_KEY",
  "countryField": "country",
  "regionField": "region",
  "cityField": "city",
  "latField": "lat",
  "lngField": "lon"
}
```

结果缓存到 SQLite `geo_cache`；查询失败不阻断访问统计。

---

## WebSocket 消息（摘要）

| type | 说明 |
|------|------|
| `auth.pow.challenge` / `auth.login` / `auth.session` / `auth.ping` | 鉴权 |
| `notes.list` / `notes.get` | 公开手账 |
| `notes.upload.*` / `notes.update.*` | 分片上传 / 更新完整 `.tnote` |
| `notes.like` / `notes.comment.create` / `notes.comments.list` | 互动 |
| `visit.track` | 访问上报 |
| `admin.*` | 后台管理 |

文件下载：`GET /files/{token}`（由 `notes.get` 签发短期 token）。

---

## 目录结构

```
TimeNotesBlog/
├── main.go / config.go
├── config.example.json # 配置模板（部署时复制为 config.json）
├── frontend/           # React 源码（仅源码构建需要）
├── web/                # 前端构建产物；go build 时 embed 进二进制
├── bin/                # 预构建发布产物
│   ├── TimeNotesBlog-2.9.0-amd64-windows.exe
│   └── TimeNotesBlog-2.9.0-amd64-linux
├── internal/
│   ├── auth/           # 密码、JWT、PoW
│   ├── protocol/       # WS 信封
│   ├── server/         # Fiber + Hub
│   ├── storage/        # Store 接口 + sqlite
│   └── geo/            # 可插拔 GeoIP
├── data/               # 运行时：数据库与 .tnote（可配置）
└── logs/               # 运行时日志（可配置）
```

---

## 开发与检查

```bash
# 后端
cd TimeNotesBlog
go test ./...
go build -o bin/timenotesblog .

# 前端（改 UI 后先 build，再 go build 才会更新嵌入资源）
cd frontend
npm run build
```

客户端相关：

```bash
cd TimeNotes
go test ./...
go build .
cd frontend
npm run build
```

---

## 默认端口

| 服务 | 地址 |
|------|------|
| Blog | `127.0.0.1:8090` |
| 客户端 Blog 桥 | `127.0.0.1:54088`（仅本机） |

---

## 常见问题

**Q: 打开首页提示「请构建前端」？**  
A: 旧版二进制未内嵌前端，或工作目录里有一份损坏/占位的 `web/index.html`。请改用 v2.9.0 预构建产物（日志应出现 `Serving frontend from embed:web/`），并删除错误的占位 `web/` 目录后重启。

**Q: 只要 exe 能不能跑？还要不要 `web/`？**  
A: 预构建产物已内嵌前端，**只需 exe + `config.json`**。`web/`、Go、Node 都不是部署依赖。

**Q: 配置 / 数据库跑丢了或写到别处？**  
A: `config.json` 与相对路径的 `data/`、`logs/` 都相对**进程工作目录**。请在固定目录启动，或用绝对路径 / `TIMENOTES_BLOG_CONFIG`。

**Q: 后台 404？**  
A: token 每次重启变化，请看最新启动日志中的 `Admin UI:` 完整 URL。

**Q: 后台「编辑」失败？**  
A: 需本机先启动 TimeNotes 客户端；桥只监听 loopback。

**Q: 上传提示文件名冲突？**  
A: 同一用户下同名 `.tnote` 拒绝新建；请用「更新到 Blog」。

**Q: 公网无法访问？**  
A: 检查 `addr` 是否绑定 `0.0.0.0`，以及系统防火墙 / 云安全组是否放行 TCP 端口。

**Q: 启动直接退出，提示 jwtSecret 太弱？**  
A: 生产必须在 `config.json` 设置足够长的 `jwtSecret`。仅本地调试可设环境变量 `TIMENOTES_BLOG_ALLOW_WEAK_JWT=1`（不要用于公网）。
