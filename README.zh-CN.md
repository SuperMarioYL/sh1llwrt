<div align="right"><sub><a href="./README.md">English</a>&nbsp;&nbsp;⇄&nbsp;&nbsp;<b>简体中文</b></sub></div>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./assets/hero-cn-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="./assets/hero-cn-light.svg">
  <img src="./assets/hero-cn-light.svg" width="880" alt="sh1llwrt — 离线本地模型，把你想要的 shell 命令写出来，无需账号">
</picture>

<p align="center"><sub>sh1llwrt 是那个离线的本地模型，把你心里想要的 shell 命令写出来，不需要账号。</sub></p>

<p align="center">
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/SuperMarioYL/sh1llwrt?color=blue" alt="license"></a>
  <a href="https://github.com/SuperMarioYL/sh1llwrt/releases"><img src="https://img.shields.io/github/v/release/SuperMarioYL/sh1llwrt" alt="最新发布"></a>
  <a href="https://github.com/SuperMarioYL/sh1llwrt/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/SuperMarioYL/sh1llwrt/ci.yml?branch=main&label=ci" alt="CI"></a>
  <img src="https://img.shields.io/badge/Go-1.24-0071E3?logo=go&logoColor=white" alt="Go 1.24">
</p>

**那个反复查 shell 参数的循环，到此为止。** 输入一句话，不到一秒拿到命令 —— 全程离线，无账号，零配置。

<h2><img src="https://api.iconify.design/tabler:topology-star-3.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 架构</h2>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./assets/atlas-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="./assets/atlas-light.svg">
  <img src="./assets/atlas-light.svg" width="880" alt="架构：$SHELL → sh1llwrt（提示词 + 缓存）→ llama.cpp（CPU / Metal）→ shell 集成（插入 / 确认）">
</picture>

一个进程、一个二进制，没有守护、没有服务、没有 socket。输入的一句话变成一个 `AskRequest`（shell、当前目录、命名文件、历史记录）；提示词模板把它组装成 shell 作用域的 few-shot 提示词；缓存的 GGUF 经 llama.cpp 推理；可解析的 `AskReply`（命令、是否安全、一句解释）回到你的 `$SHELL`。首次运行会一次性下载 941MB 的 Qwen2.5-Coder-1.5B Q4_K_M GGUF 并校验 sha256；此后每一次运行都完全离线、无账号、亚秒级。

<h2><img src="https://api.iconify.design/tabler:bulb.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 为什么有这个项目</h2>

从意图到一条正确、贴合文件的 shell 命令，如今要四步手动加一个浏览器标签页，而它本该是零步。每周你都重新查 `tar` / `ffmpeg` / `magick` / `sed` / `kubectl` / `rsync` 的参数，终端上下文被浏览器打断，粘回来的命令还不知道你真实的 `cwd` 和文件名。sh1llwrt 把这一整套压成 `$SHELL` 里的一句话 —— 离线、亚秒、无账号。痛点不是「shell 命令难」，而是「意图到命令的往返根本不该离开终端」。

> **诚实的质量下限。** 这个 OP 级模型在 InterCode-ALFA 上 0.620（GPT-4o 为 0.73）。真正抬升的是 shell 作用域的提示词，不是权重。如果手编的 20 题开发任务基准低于 0.70，v0.1 不再推进 —— 见路线图的 kill 条款。

<details>
<summary>目录</summary>

- [架构](#-架构)
- [为什么有这个项目](#-为什么有这个项目)
- [快速开始](#-快速开始)
- [用法](#-用法)
- [Demo](#-demo)
- [配置](#-配置)
- [路线图](#-路线图)
- [License](#-license)
</details>

<h2><img src="https://api.iconify.design/tabler:rocket.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 快速开始</h2>

冷克隆到第一条命令，三步：

```bash
git clone https://github.com/SuperMarioYL/sh1llwrt && cd sh1llwrt
go install ./cmd/sh1llwrt          # 编译二进制到 $GOPATH/bin
sh1llwrt init && exec $SHELL        # 接入 ? 前缀；重启 shell
```

然后提问（首次运行一次性下载 941MB 模型，之后永久离线）：

```bash
? extract foo.tar.gz to /tmp
# → tar -xzf foo.tar.gz -C /tmp
```

想用 `curl|sh`？（首个发布打 tag 后可用）：

```bash
curl -fsSL https://raw.githubusercontent.com/SuperMarioYL/sh1llwrt/main/scripts/install.sh | sh
```

<details>
<summary>示例输出</summary>

```
$ ? resize cat.png to 200x200
# resize image to 200x200 box
magick cat.png -resize 200x200 cat_small.png
# destructive — review before pressing Enter
```
</details>

> **还没下载模型？** `sh1llwrt ask --mock "<意图>"` 在一个确定性 mock 后端上跑完整的 提示词→解析→打印 流程，不需要 GGUF。它用于测试、demo gif，以及免下载体验交互。

<h2><img src="https://api.iconify.design/tabler:terminal-2.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 用法</h2>

```bash
# 核心动词 —— 把一句话变成一条命令
sh1llwrt ask "convert clip.mov to mp4 h264"
# → ffmpeg -i clip.mov -c:v libx264 -c:a aac clip.mp4

# 把你真实的 cwd + 命名文件注入为上下文
sh1llwrt ask --cwd "$PWD" --glob img.png,img.tiff "resize the png to half"

# 把 ? 前缀接入 zsh/bash（幂等 —— 重复运行会替换原块）
sh1llwrt init

# 机器可读输出，便于脚本 / 包装
sh1llwrt ask --json "list kube-system pods"
# → {"command":"kubectl get pods -n kube-system","safe":true,"explain":"read-only pod listing"}
```

主要标志位：`--shell zsh|bash`、`--cwd`、`--glob`（可重复）、`--threads`、`--max-tokens`、`--temperature`、`--mock`、`--json`、`--insert`（m2，m1 阶段失败即关闭）、`--verbose`。详见 `sh1llwrt ask --help`。更多示例见 [`examples/`](./examples)。

<h2><img src="https://api.iconify.design/tabler:photo.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> Demo</h2>

下面的 gif 用 `--mock` 模式运行真实二进制，所以能免一次性 941MB 下载直接渲染；真实首次运行除了那一次下载，其余完全一致。

![demo](assets/demo.gif)

生成它的 tape 在 [`docs/demo.tape`](./docs/demo.tape)，可通过 `demo` 工作流按需重渲染。

<h2><img src="https://api.iconify.design/tabler:adjustments.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 配置</h2>

sh1llwrt 没有配置文件 —— 所有旋钮都是环境变量。v0.1 默认零配置。

| 环境变量 | 默认值 | 含义 |
|---|---|---|
| `SH1LLWRT_CACHE_DIR` | `$XDG_CACHE_HOME/sh1llwrt` | GGUF 缓存根目录（气隙部署可改写） |
| `SH1LLWRT_MODEL_URL` | HuggingFace 规范地址 | 改写一次性下载 URL |
| `SH1LLWRT_MODEL_SHA256` | *(未设)* | 期望 sha256；未设 = 信任已有文件（发布前） |
| `SH1LLWRT_LLAMA_CLI` | PATH 中的 `llama-cli` | llama.cpp CLI 二进制路径 |
| `SH1LLWRT_MOCK` | *(未设)* | `1` = 用确定性 mock 后端（无需模型） |

> 打 `v0.1.0` tag 前，把 `internal/model/cache.go` 里的 `DefaultModelSHA256` 设为规范 GGUF 的实测摘要（或传 `SH1LLWRT_MODEL_SHA256`）。在此之前 sha 校验会跳过并告警，以保证二进制可端到端运行。

<h2><img src="https://api.iconify.design/tabler:map-2.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 路线图</h2>

- [x] **m1_boot_model** —— `sh1llwrt ask` 经 llama.cpp 在 <1s（热缓存）返回命令；首次运行自动下载 + sha256 校验 GGUF 到 `$XDG_CACHE_HOME/sh1llwrt/`。
- [ ] **m2_shell_meet** —— `sh1llwrt init` 把 `?` 前缀 + Ctrl-Space 热键接入 zsh/bash，一句话变成可插入的光标处命令，对破坏性动词做 Safe-confirm。
- [ ] **m3_ship_binary** —— darwin/arm64 + linux/amd64 的静态 CGO 二进制；`curl|sh` 安装器；README + 60s demo gif。
- [ ] **未来** —— 手编 20 题开发任务基准（kill 门槛：<0.70 即停线）；oh-my-zsh 插件；kubectl/git/docker 参数扩展；Windows。

**Kill 条款（来自计划）：** 发布后 21 天，若 GitHub star <100 且自然 issue <3 且热缓存 p95 >1.2s（i5 级 / 4 线程笔记本）—— 或手编 20 题基准正确率低于 0.70 —— 则不再推进。

<h2><img src="https://api.iconify.design/tabler:license.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> License</h2>

MIT —— 见 [LICENSE](./LICENSE)。Issue 与 PR 提到 [GitHub 仓库](https://github.com/SuperMarioYL/sh1llwrt/issues)。v0.1 仅开源、免费、无托管层、无付费功能。

<p align="center"><sub><a href="./LICENSE">MIT</a> © 2026 SuperMarioYL</sub></p>
