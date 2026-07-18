# CyberAgent Workbench Desktop Plan

状态：Desktop D0-A、D0-B 与 D1-R1 至 schema-v77 D1-K1 自动化核心已完成。Wails v2.13.0 Windows 壳、嵌入式 React bundle、进程内 Go API、同库恢复、高水位事件续传、WebView2 失败关闭、内存令牌、原生 `.zip` 对话框、路径隔离 Skill 预览/惰性安装、受控 Run/Session/Plan/审批、显式无正文 Provider 诊断、持久化模型路由、Diff 独立审阅/apply、有界 wake/retry 意图/显式前台消费、统一操作回执与历史、Workspace Files/search、非授权 evidence 附加/清单、操作者行动中心、`Ctrl+K` 命令面板和可复现便携构建诊断已经落地；Windows 10 实机矩阵、Desktop Provider 密钥设置、后台 wake worker、安装包、签名正式发行、注册表、自启动、更新和高权限执行仍未实现。

## 目标

在保留 `cyberagent` CLI 为一等入口的同时，提供面向日常使用的 Windows 桌面端。桌面端参考 Codex 的项目/任务线程、活动流、Diff 审查、审批队列和多 Agent 观察体验，但使用 CyberAgent Workbench 自己的 Code/Cyber、Plan/Deliver、Run、Policy、Skill 和 Sandbox 模型，不复制其视觉资产或私有实现。

## 架构决定

- Go 仍是唯一控制平面；Policy、Scope、预算、SQLite、模型、工具、文件、Docker、Shell 和密钥均由 Go 管理。
- React/Vite 复用现有 `web/` 控制台；桌面外壳已固定 Wails v2.13.0 稳定版。v3 仍处于 Alpha，不进入当前主线。
- TypeScript 业务操作继续使用 Go 拥有的 HTTP/OpenAPI 契约；普通 Web 保留 SSE，Windows Desktop 因 Wails v2 AssetServer 不支持流式响应而使用同一事件表、同一 Run-bound 高水位 cursor 的 `run-event-poll.v1`。桌面绑定只处理内存启动材料、受控文件选择和一次性句柄确认安装，不建立第二套业务 API。
- Electron 不作为默认方案，避免 Node 成为并行高权限控制面。Tauri 不作为默认方案，避免 Rust 从确定性分析工具变成桌面主控。
- 桌面端不直接读取 API key，不直接控制 Docker/Shell，不实现 Scope 或文件权限判断，也不在 Local Storage、前端日志或注册表保存凭证。
- CLI、TUI、Web 和 Desktop 必须投影同一 Run/Event/Approval 状态；关闭窗口不得隐式取消后台 Run。D0-A/D0-B 只读取已有状态，D1-R1 创建关闭的 Run 图，D1-S1/S2 控制现有 v45-v46 队列，D1-L1 控制生命周期，D1-X1 才能通过 Go RunSupervisor 短暂持有私有 execution lease。TypeScript 和 native bridge 永远不持有 lease。

## 目标布局

- 左栏：Project、Workspace、Run、Session 与任务状态。
- 中栏：对话、执行进度、事件流、排队输入和交付结果。
- 右栏：Plan、WorkItems、Diff、Agents、Tools、Approvals、Findings 与 Artifacts。
- 底部：输入区、Code/Cyber 工作面、Plan/Deliver 阶段、Provider/Model 和预算摘要。
- 独立页面：Skills、Providers、Policy、Workspace、审计记录和桌面设置。

所有高风险操作必须显示 Go 返回的结构化候选、Scope、风险代码和审批要求；前端文案不能自行推导授权结果。

## 已完成的前置边界

- `skills.ReadPackageFile` 已成为 CLI 与未来 Desktop 共用的有界普通文件读取器；它拒绝 symlink、目录、空/超限包和首尾空白改写路径，并在读取前后复核身份且不回显路径。
- `desktop.NewSkillPackagePreviewBoundary` 把未来原生选择器和渲染桥拆成两个值：只有 Go selector 接收路径，renderer bridge 只接收 256-bit 不透明句柄。
- Go 在发放句柄前完成严格包校验并立即丢弃路径/正文；内存最多保留 16 份投影，五分钟过期且单次消费。
- `desktop_skill_package_preview.v1` 只返回有界风险元数据，排除路径、文件名、正文、Manifest description/content path/content digest，并固定安装、命令、网络、Provider、工具和能力授权为 false。
- D0-A 已把该边界接入 Wails 原生对话框和 React 只读预览；D1-B1 再允许渲染层提交一次性确认句柄，由 Go 重新消费同一已验证包并写入惰性 Registry。渲染层仍不能提交路径或文件字节，安装不会执行包内容、选择 Run 或授予能力；ADR 0033、ADR 0034 与 ADR 0041 记录这些边界。

## D0-A 至 D1-K1 当前实现

- `cmd/cyberagent-desktop` 只在 Windows `desktop,wv2runtime.error` build tags 下编译，production 构建再增加 `production`；默认 read-only。`--enable-profile-control`、`--enable-run-creation`、`--enable-session-messages`、`--enable-session-steering-control`、`--enable-run-lifecycle`、`--enable-run-execution`、`--enable-plan-delivery`、`--enable-approvals`、`--enable-model-control`、`--enable-file-edit-review`、`--enable-run-wake`、`--enable-file-edit-apply`、`--enable-run-wake-execution`、`--enable-skill-installation` 和 `--enable-evidence-attachments` 分别开放窄 Go route；十五项 capability 独立，单项启用不能访问其他 route。模型可用性、Workspace search、receipt history、operator actions 和 evidence inventory 只使用 read token；`Ctrl+K` 只在客户端导航或刷新这些读取。
- `web/dist` 以 compile-time embed 进入二进制；Go 在启动前验证 index、内容哈希资源、类型、数量、单项/总大小并复制为不可变内存快照。
- Wails AssetServer 直接调用现有 `httpapi.API` Handler，不监听 TCP 端口；同一 Go 层继续负责 Bearer、Host、CSP、Policy、SQLite 和 DTO。
- Renderer 绑定面只有 `Bootstrap`、`SelectSkillPackage`、`PreviewSkillPackage`、`InstallSkillPackage` 四个方法。最后一项只消费 Go 发放的短期确认句柄；renderer path/bytes、进程、Shell、Docker、安装时执行和能力授予全部不可达。
- read/control token 每次进程启动随机生成，只驻留内存，不写 Local Storage、SQLite、日志、命令输出或注册表；两者不得相同。只有至少一个显式 control capability 开启时才生成 control token。
- 单实例、窗口恢复、禁用 WebView 文件拖放/默认右键菜单、renderer code integrity、路径隔离错误和 bounded startup dialog 已接入。
- `desktop.ControlPlane` 与 `desktop.Lifecycle` 固定同库 API 所有权、幂等关闭、崩溃重开、第二实例让位和停止后永久静默；第二实例参数与工作目录不会进入主实例。
- Desktop 通过 `GET /api/v1/runs/{run_id}/events/poll` 消费与 SSE 相同的真实事件 frame/cursor；React 最多在内存保留 16 个 Run、每个 500 帧，不写浏览器存储。
- Run Files 页通过 read bearer 调用 Go-owned `workspace_explorer.v1`；renderer 只使用 Go 返回的 canonical 相对子路径，不能提交 host root，内容经过有界 UTF-8/secret redaction 并标记为 non-authorizing evidence。
- Files 页的 `workspace_search.v1` 只扫描 Explorer 脱敏投影；独立 evidence capability 启用后，renderer 也只能提交 Go 返回的相对引用与 SHA-256。Go 重新投影并以 `instruction_authorized=false` 原子附加到既有 Session，不调用模型/工具。
- FileEdit apply、foreground wake consume 与 inert Skill install 使用同一 `operation_receipt.v1`；React 交叉校验父响应，回执不含 operation key/digest、路径/正文或 private lease。
- Receipts 页通过 `operation_receipt_history.v1` 显式刷新终态历史；只返回最多 100 条 metadata，FileEdit staging 只读检查且不执行清理。
- Actions 页通过 `operator_action_center.v1` 聚合最多 100 条闭集 pending steering/approval/FileEdit/due-wake metadata；Go 重检 exact Run/Mission/Session/Workspace，不返回正文、命令、路径、Diff、私有 operation/lease，也不自动处理。
- Evidence 页通过 `session_evidence_inventory.v1` 列出 exact Run/Session 已附加来源/hash/time 与固定 false authority；source navigation 只复用 Go-issued canonical reference 和既有 Explorer。
- `Ctrl+K` 命令面板只有静态导航/刷新命令，不提交路径、正文、审批、operation、capability、进程或密钥。SSE 客户端与 Go/OpenAPI 共用 literal `v1`，失败重连会先 cancel response reader，避免耗尽 WebView2 连接。
- WebView2 `94.0.992.31` 以上只读预检发生在 bundle/数据库之前；失败时不下载、不安装、不打开 URL。进程内适配器只接受精确 `http://wails.localhost`，外部链接、表单和 popup 在 Desktop renderer 中被阻止。
- secure production-tag 二进制已经在隔离数据目录通过 Windows 11 强制结束/重开与第二实例实机烟测；主工作台、Skill modal 与原生 `.zip` 对话框也已通过视觉复核。D1-R1 至 D1-A1 的 route、能力分离、重放和 React 交互由自动化覆盖，正式发布前仍需随最终二进制复跑完整 Windows 10/11 人工矩阵。

本地构建：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-desktop.ps1
# 发布候选诊断会连续构建两次并比较 SHA-256。
powershell -ExecutionPolicy Bypass -File scripts/build-desktop.ps1 -VerifyReproducible
go run ./cmd/cyberagent doctor portable --json
```

输出位于 `build/desktop/cyberagent-desktop.exe`，默认不开放 control token。构建目录必须留在仓库且不能穿过 child reparse point；双构建会输出忽略的 release/compatibility JSON。该文件是未签名开发/便携测试产物，不是正式发行包。机器需要 Windows 10/11 和 WebView2 Evergreen Runtime `94.0.992.31` 或更新版本；缺失时应用只给出本机指导，不会隐式安装。测试其他数据目录时，可在启动前设置 `CYBERAGENT_HOME`；桌面渲染层无法读取或更改这个路径。自动检查通过后 `release_ready` 仍为 false，直到 Windows 10/WebView2 人工矩阵完成。

显式启用受控 Run 创建：

```powershell
.\build\desktop\cyberagent-desktop.exe --enable-run-creation

# 可与非授权档位选择共同启用。
.\build\desktop\cyberagent-desktop.exe --enable-run-creation --enable-profile-control
```

创建只接受已注册 Workspace，并固定默认预算、禁用网络/目标和 `preview/noop` 执行档位；它不会自动发送 Session 消息、调用模型、取得 execution lease 或启动进程。

显式启用 Session 消息排队：

```powershell
.\build\desktop\cyberagent-desktop.exe --enable-session-messages

# 三项 capability 可组合，但仍不授予执行权。
.\build\desktop\cyberagent-desktop.exe --enable-run-creation --enable-profile-control --enable-session-messages
```

消息提交只接受精确绑定到 running/paused Run 的现有 Session，按既有 v45-v46 规则脱敏、持久化和幂等重放。它不会启动 created Run、恢复 paused Run、drain 队列、调用模型/工具或取得 lease。

显式启用队列取消、Run 生命周期和有界执行：

```powershell
.\build\desktop\cyberagent-desktop.exe --enable-session-steering-control
.\build\desktop\cyberagent-desktop.exe --enable-run-lifecycle
.\build\desktop\cyberagent-desktop.exe --enable-run-execution
```

取消只适用于未 prepared 的 pending 项。生命周期只执行严格 start/pause/resume。执行入口冻结最多八条 pending 身份，并通过现有 RunSupervisor、Policy、预算、模型/工具账本和私有 lease 消费；它不是 Desktop-native worker，也不能启动 Shell、Local 或 Docker 进程。

显式启用 Plan/Deliver 与审批决策：

```powershell
.\build\desktop\cyberagent-desktop.exe --enable-plan-delivery
.\build\desktop\cyberagent-desktop.exe --enable-approvals
```

Plan 选择只消费已持久化的三方向提案并创建既有 WorkItem/Note 事实，进入 Deliver 必须第二次显式操作。审批队列不返回命令、路径、文件内容、指纹或原因；approve-once 会重检 Policy，且只能得到 dry-run Shell 或 process-disabled ScriptProcess 结果。文件替换不能通过该入口批准，永久拒绝不能覆盖，所有进程/文件写入/Grant 权限仍为 false。

显式启用 Diff apply、一次前台 wake 消费或惰性 Skill 安装：

```powershell
.\build\desktop\cyberagent-desktop.exe --enable-file-edit-apply
.\build\desktop\cyberagent-desktop.exe --enable-run-wake-execution
.\build\desktop\cyberagent-desktop.exe --enable-skill-installation
```

三项能力彼此独立，也独立于 Diff review、wake intent 和 Skill preview。Apply 只写已精确批准且当前 hash/Policy 仍匹配的 Workspace 文件；wake 只在点击后通过既有 RunSupervisor 消费一条到期 intent；Skill 安装只把已预览包登记为 `operator_installed_untrusted`，不执行或自动选择它。任何一项都不会启动后台 worker 或通用宿主/容器进程。

显式启用非授权 Workspace evidence 附加：

```powershell
.\build\desktop\cyberagent-desktop.exe --enable-evidence-attachments
```

该 flag 只开放一个精确 Run/Session/Workspace/hash 绑定的附件 route。Workspace 搜索和回执历史仍是 read token 能力；附件文本以 tool-role 持久化，但投影给模型时固定为 untrusted user evidence，不能授权工具、进程、网络或文件写入。

## 分阶段交付

### D0：桌面基础验证（自动化核心完成，Windows 10 实机待补）

- [x] 固定 Wails v2.13.0 和 `cmd/cyberagent-desktop` Windows build-tag 边界。
- [x] 嵌入现有 React read-first 控制台，以进程内 Handler 复用 Go API/OpenAPI；v64 档位选择仍只调用 Go control route。
- [x] 建立随机内存令牌、单实例、窗口恢复、生产 bundle、CSP 与原生 `.zip` 对话框；渲染层只消费一次性句柄。
- [x] 输出未签名开发/便携测试二进制，不创建安装器、注册表、自启动、更新、协议关联或后台服务。
- [x] D0-B 完成 CLI/Desktop 同库并发、关闭/崩溃重开、单实例恢复、poll/SSE cursor 互续与 WebView2 缺失/过旧/探测失败诊断。
- [x] D0-B 增加精确 renderer origin、规范 `RequestURI`、外部 navigation/form/popup 阻断、secure build-tag 门禁、Windows CI 和 Windows 11 实机恢复记录；仍不增加业务 mutation。
- [ ] 在正式便携或签名发行前补齐 Windows 10 x64 实机启动、第二实例、强制结束/重开和 WebView2 缺失路径矩阵。

### D1：日常工作台（产品可用度约 80-84%）

- [x] D1-R1 / schema v72：Go API 受控创建 Mission/Run/Session，严格注册 Workspace、Scope、默认预算、幂等 operation、事务事件和关闭 execution profile；React 可选择 Workspace/Profile/Surface/Phase 并在成功后刷新、选中新 Run。
- [x] D1-R1 capability 与 `--enable-profile-control` 独立；creation-only token 不能访问旧控制 route，Wails native bridge 不增加方法。
- [x] D1-R1 完整门禁通过全仓普通/race、普通/secure-Desktop 检查、45 项前端测试、确定性契约、生产构建、依赖/隐私/Markdown 扫描和隔离 CLI smoke；没有已知未解决高/中风险。
- [x] D1-S1：三个切片完成 Go/Application 与严格 HTTP message submission、独立 Desktop bootstrap capability、React composer/内存重试/metadata-only 反馈；复用既有队列而不启动执行。
- [x] D1-S1 功能门通过全仓普通 Go、Desktop-tag 聚焦、52 项前端测试、严格 TypeScript、Vite 与 Windows production build；完整六切片健壮性门排在下一批结束。
- [x] D1-S2：独立 pending-only steering cancellation control/UI 已完成；`prepared` 只读派生，不能取消 prepared/committed/cancelled 项。
- [x] D1-L1 / schema v73：幂等 operator Run start/pause/resume 已完成，独立复核状态、quiescence、lease、Agent/Supervisor 和 capability。
- [x] D1-X1 / schema v73：Go-owned bounded execution handoff 已通过既有 Supervisor/预算/Policy/lease/model/tool/event 路径消费冻结队列，不建立 native executor。
- [x] D1-X1 后累计六片完整健壮性门已通过：ordinary/race 268.2 秒/295.3 秒、vet、staticcheck、govulncheck、依赖/隐私、确定性契约、66 项前端测试、Windows/Vite build、重启与并发功能复核均为绿色。
- [x] D1-M1：CLI/API/Desktop 已统一使用 Go Provider Registry/持久化模型路由；renderer 只取得脱敏可用性，API key、Base URL 和环境变量名不进入 DTO，也不触发探测。
- [x] D1-P1：已增加独立 Plan 三选一与显式 Plan-to-Deliver control；模型不能代选，选择不能自动切换阶段或执行。
- [x] D1-A1：已增加 durable approval queue 的 metadata-only 投影和 approve-once/deny；永久拒绝、文件写入、Session Grant 与真实进程不可覆盖。
- [x] D1-M1/P1/A1 三切片普通功能门通过全仓 Go、Desktop tag、73 项前端测试、strict TypeScript、OpenAPI、Vite/Windows production build 和 npm audit；组合审计无已知高/中风险。
- [x] D1-M2：增加显式 content-free Provider 诊断和 persist-before-memory 路由选择；状态 DTO 不含模型正文、密钥、端点、环境变量名或原始错误。
- [x] D1-D1：增加 exact-bound metadata-only FileEdit 队列、脱敏 Diff 与 approve-intent/deny；文件正文不进 DTO，批准不写文件。
- [x] D1-Q1 / schema v74：增加可取消的有界 wake/retry 意图、deadline/backoff 和单 owner fencing；公开 DTO 不含 lease owner，后台 loop/执行权固定为 false。
- [x] D1-M2/D1-D1/D1-Q1 后累计六片完整健壮性门已通过：最终 ordinary/race 278.6/296.1 秒、双路径静态/漏洞检查、80 项前端测试、确定性契约、Windows/Vite production build、CLI smoke 与仓库隐私复核均为绿色。
- [x] D1-Q2 / schema v75：显式前台 wake 消费已复用既有 handoff/RunSupervisor/预算/Policy/取消/fencing；没有隐藏 worker，未知 in-flight handoff 保持 prepared。
- [x] D1-D2 / schema v76：已批准 Diff 的独立 apply 已完成新鲜 Policy、Workspace path、当前/目标 hash 与幂等写入复核；renderer 不提交路径或正文。
- [x] D1-B1：一次性句柄 Desktop 安装与 canonical-base64 HTTP 安装已复用惰性 Registry；不执行包内容、脚本/钩子、网络、Provider 或工具，也不选择到 Run。
- [x] D1-Q2/D1-D2/D1-B1 三切片普通功能门通过：最终 Go 333.1 秒、聚焦 race、Windows Desktop tag、85 项前端测试、strict TypeScript、确定性契约、Windows/Vite build、vet/module/npm/CLI smoke 与仓库卫生均为绿色；完整静态/漏洞门在下一批累计六片后执行。
- [x] D1-U1：统一 `operation_receipt.v1` 与 FileEdit 暂存恢复；回执无正文/路径/private identity，失败与 pending cleanup 不再显示为成功。
- [x] D1-E1：Go-owned bounded Workspace explorer 与 Files 页；canonical relative path、link/redirect 拒绝、400/200 entry、64/128 KiB input/projection、root/staging 隐私和 evidence-only provenance 均已固定。
- [x] D1-W1：portable doctor、reproducible linker metadata、连续双构建 SHA-256、PE/零 COFF timestamp/trimpath/module/non-installing checklist 与 PowerShell 5.1 兼容已完成；人工 Windows 10 矩阵仍待补。
- [x] D1-U1/E1/W1 后累计六片完整健壮性门已通过：ordinary/race 294.0/338.3 秒、普通/secure-Desktop test/vet、staticcheck、govulncheck、module/依赖/隐私、88 项 React、确定性契约、Vite 与真实 Windows 双构建均为绿色；双构建 SHA-256 为 `33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`，无已知未解决高/中风险。
- [x] D1-E2：有界 Workspace filename/redacted-text search 已完成；硬上限、无 link/indexer、canonical relative reference 与 false-authority provenance 已固定。
- [x] D1-C1 / schema v77：操作者显式 evidence attachment 已完成；独立 default-off capability、精确 hash/binding、原子 message/event/attachment 和 SQLite false-authority trigger 已固定。
- [x] D1-U2：refreshable metadata-only receipt history 已完成；最多 100 条、exact Run filter、opaque ID、无 operation/path/private lease，staging inspection 只读。
- [x] D1-E2/C1/U2 普通功能门通过：Go 297.9 秒、Desktop tag、92 项 React、strict TypeScript、vet/module、确定性契约、Vite/Windows 可复现构建和 npm 零漏洞均为绿色；无已知未解决高/中风险。
- [x] D1-O1：bounded Go-owned operator action center 已完成；最多 100 条 closed metadata、exact binding、opaque ID，不自动审批/执行。
- [x] D1-C2：metadata-only attached-evidence inventory 已完成；正文/private identity 不出 Go，false instruction authority 不可推导扩权。
- [x] D1-K1：existing-view navigation/refresh-only `Ctrl+K` command palette 已完成；没有 renderer host path、mutation 或进程入口。
- [x] D1-O1/C2/K1 后累计六片完整健壮性门通过：ordinary/race 319.6/299.8 秒、ordinary/secure-Desktop test/vet、staticcheck、govulncheck、module/依赖/隐私、97 项 React、确定性契约、Vite/Windows 可复现构建和真实浏览器桌面/移动复核均为绿色；审计修复事件 literal `v1` 漂移与失败重连连接泄漏，无已知未解决高/中风险。
- [ ] 下一批候选 D1-I1 Go-issued Monaco proposal/Diff editor、D1-M3 Go/OS-owned Provider secret boundary、D1-J1 default-off bounded wake worker；先分别完成威胁模型，renderer 不直接写文件或读回明文密钥，worker 不启动 Shell/Local/Docker。
- [ ] 所有状态 mutation 使用独立 control capability、Origin/Host 校验、稳定 operation key 和 typed errors；显式 Provider 诊断每次只允许一次有界无正文请求。CLI/Desktop 并发、窗口重开、后台 Run、重放与断线续传不得只沿用 D0 结论。
- [ ] Code 与 Cyber 保持不同 Skill 目录和风险呈现；桌面切换不改变 Run 内不可变模式。

### D2：Windows Beta 分发（发布成熟度阶段）

- 提供便携 ZIP 和签名 MSIX；检测 WebView2 Evergreen Runtime，缺失时使用受控引导或依赖安装。
- 固定 per-user 安装、升级、降级、卸载、用户数据保留/删除和崩溃日志策略。
- 代码签名材料只进入受保护 CI secret/签名服务，不进入仓库或普通构建日志。
- 发布产物生成 SBOM、哈希、签名和可复现版本元数据；自动更新仍需独立安全审查。

### D3：按需扩展

- 企业 MSI、Microsoft Store、远程环境、`cyberagent://` 协议、文件关联、开机启动和后台服务分别立项。
- 任何自动更新必须验证签名、版本通道、防降级策略、原子替换和失败回滚。
- macOS/Linux 打包只在 Windows Beta 稳定后评估，不让跨平台包装阻塞 Go 核心能力。

## 安装与注册表边界

- 开发和便携版不要求安装包，也不要求应用自定义注册表键。
- MSIX/MSI 可由系统/安装器登记应用身份和卸载信息；业务状态、凭证和 Policy 不存注册表。
- 文件关联、自定义协议、自启动、服务和右键菜单会扩大系统权限面，D0-D2 默认不启用。
- 安装器不得请求管理员权限，除非未来某个独立企业场景给出不可替代的理由并通过审计。

## 发布门禁

- Windows 自动化覆盖启动前置条件、关闭/恢复、同库续传和 renderer 边界；Windows 11 x64 已实机验证，Windows 10 x64 仍是正式发行前必过矩阵。
- 渲染进程无法绕过 Go API 访问 Shell、Docker、密钥或工作区外文件。
- CLI 与 Desktop 同时运行时，SQLite 不损坏、operation 重放不分叉、Run lease 不被窗口生命周期或 Run 创建偷取。
- 安装、升级和卸载不会静默删除 Workspace、数据库、凭证或用户创建文件。
- 未签名开发产物不得伪装成正式发布；正式包必须有可核验签名和哈希。

ADR 0034 至 ADR 0044 分别记录可见壳、生命周期/事件续传、受控 Run 创建、Session message submission、Run 控制/有界交接、模型/Plan/审批、Provider/Diff/wake intent、前台 wake/独立 apply/惰性 Skill 安装、统一回执/Workspace evidence/便携构建诊断、Workspace search/non-authorizing attachment/receipt history，以及 operator actions/evidence inventory/command palette。Wails 使用 MIT 许可证；D2 生成任何可分发 ZIP/MSIX 前必须把 Wails 及其他运行时依赖的许可证/notice、SBOM 和哈希一起打包。
