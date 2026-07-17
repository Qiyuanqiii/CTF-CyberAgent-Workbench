# CyberAgent Workbench Custom Skill Package Plan

状态：`skill_package.v1` 纯内存校验与 schema v69 内容寻址本地 Registry 已完成；CLI 可显式确认导入、查询和追加移除 tombstone。外部 Run 选择/上下文加载、HTTP/桌面上传、签名与 Marketplace 尚未实现。

## 当前能力

现有 Go `skill.v1` 已严格定义名称、语义版本、描述、兼容 Profile、工具前置声明、Markdown 内容路径、UTF-8 字节数、保守 token 上界和 SHA-256。`internal/skills.LoadFS` 可以从受控 `fs.FS` 构造不可变 Registry，当前只用于编译进二进制的 `code`、`review`、`learn`、`script` 和 `plan-delivery`，以及测试夹具。

当前产品入口包括：

- `cyberagent skill list`
- `cyberagent skill show`
- `cyberagent skill validate`
- `cyberagent skill package validate <package.zip>`
- `cyberagent skill import <package.zip> --surface code|cyber --operation-key <stable-key> --confirm-untrusted-skill`
- `cyberagent skill installed [--surface code|cyber] [--profile <profile>] [--include-removed]`
- `cyberagent skill installed show <name>@<version>`
- `cyberagent skill remove <name>@<version> --operation-key <stable-key> --confirm-remove`
- `cyberagent skill select`
- `cyberagent skill selection`

其中 `skill package validate` 只通过有界普通文件读取和纯内存 parser 返回 metadata-only 风险预览，不创建数据库、不落盘、不安装、不执行正文，也不访问网络、Provider 或工具。schema v69 的 `skill import` 在显式确认后只增加不可变元数据与原始包对象，仍不执行正文、联网、调用 Provider/工具或授予能力。项目目前没有外部 Run 选择、签名、HTTP 上传端点或桌面文件选择入口；内部 `LoadFS` 仍不能被描述为用户导入功能。

## 第一版包边界

- `skill_package.v1` 第一版只允许一个严格 `skill.v1` Manifest 和一份 UTF-8 Markdown 指导正文。
- ADR 0024 已固定确定性 ZIP：根目录按顺序只能包含 `manifest.json`、`SKILL.md`，两项都使用 Deflate、零时间戳、无 extra/comment/属性/前缀/间隙/尾随数据，并使用固定 ZIP 2.0 data-descriptor profile。
- archive 最大 64 KiB，文件数固定为 2；解压总量、单项大小与压缩比均有硬上限。`archive_sha256` 标识原始容器，`package_fingerprint` 通过协议版本、规范 Manifest 和正文的长度分帧标识语义内容。
- 禁止绝对路径、`..`、反斜杠歧义、重复/大小写碰撞条目、symlink、hardlink、设备、FIFO、嵌套压缩包、尾随数据和 ZIP bomb。
- 限制压缩包字节、解压字节、文件数、目录深度、单文件大小和压缩比；所有限制在读取正文前执行。
- 第一版不接受可执行脚本、二进制、动态库、任意资源下载或安装钩子。后续脚本资源仍只能作为 Artifact，由 Go Tool Gateway、Policy、Scope 和审批独立授权。

## 信任与权限

- 包内容是操作者安装的工作流指导，不是系统策略，也不能覆盖 Policy、Scope、预算、Approval、Sandbox 或来源隔离。
- `tool_dependencies` 仍只是前置声明，永不授予工具能力。
- 导入不会执行正文、脚本、命令、网络请求或 Provider 调用。
- 外部包固定标记为 `operator_installed_untrusted`；schema v69 只允许操作者显式安装，尚无任何外部包进入模型上下文的路径。
- 上下文交付仍需版本/hash/bytes/Profile 精确复核、secret redaction、独立 token 预算和 capability=false 来源事实。
- Code 与 Cyber Catalog 分离。Cyber 默认不继承 Code Skill；Script Profile 只能使用明确兼容的窄化脚本指导。

## 本地 Registry

- Go 在用户数据目录的 `skill-registry/objects/sha256/<prefix>/<digest>.zip` 维护 content-addressed 对象；正文不进入仓库、SQLite 或 Run 事件。
- SQLite 只保存包摘要、信任状态、Profile、安装/完成操作和移除 tombstone 等有界元数据；五类表全部不可更新或删除。
- 相同对象并发发布收敛；同名同版本已有不可变安装时冲突；Registry 最多 64 个历史包身份，同名最多 8 个版本。
- 已被 Run 精确固定的版本不能移除。移除只追加 tombstone 且保留对象；恢复/重装需要未来显式协议，不静默推断。
- 导入先写安装意图，再使用同目录独占临时文件、file sync、原子 hard link 和完整回读验证发布对象，最后写完成结果；崩溃后以同一 operation key 恢复。目录 sync 在 Windows 上为 best effort，不作超出平台证据的持久性声明。
- 对象接口只有 `Put` 与 `Verify`，没有执行或删除；每次完成重放、列表和详情读取都重新验证字节数、archive SHA-256、ZIP 结构与语义指纹。

## 产品入口

CLI 第一阶段：

```text
cyberagent skill package validate <package.zip>
cyberagent skill import <package.zip> --surface code|cyber --operation-key <stable-key> --confirm-untrusted-skill
cyberagent skill installed [--surface code|cyber] [--profile <profile>]
cyberagent skill installed show <name>@<version>
cyberagent skill remove <name>@<version> --operation-key <stable-key> --confirm-remove
```

Desktop D1 阶段：

- 文件选择器只把一个受限本地文件句柄/路径交给 Go；TypeScript 不解压、不校验、不写 Registry。
- Go 先返回 metadata-only 验证预览、风险代码、Profile、声明工具和 package digest，再要求显式确认安装。
- 上传端点只能绑定回环地址，使用独立 control token、严格 Origin/Host、固定 Content-Type、流式大小上限、幂等 operation key 和无正文审计事件。
- Web 远程上传、URL 安装、Git 仓库安装和在线 Marketplace 均后置，不允许第一版自动下载并信任内容。

## 签名与分发

- 本地未签名包可以在明确警告后安装，但保持 untrusted 标记。
- 签名包需要独立 publisher identity、签名算法、证书轮换、吊销和透明来源设计；签名只证明发布者和完整性，不授予能力。
- 官方内置 Skill 与用户安装 Skill 使用不同 trust class 和 Catalog，不允许用户包覆盖内置名称/版本。

## 实施切片

1. [x] 固定 `skill_package.v1` 和威胁模型，完成纯内存 parser/validator/fuzzer 与 metadata-only CLI 校验，不写磁盘。
2. [x] schema v69 增加 content-addressed 本地 Registry、不可变安装/移除账本、原子导入/恢复和 CLI；导入保持惰性。
3. [ ] schema v70 将用户 Skill 纳入 Run 的精确版本选择、root/Specialist 最小上下文和 Code/Cyber 分离测试。
4. [ ] 在 Desktop D1 增加 Go-owned 文件选择、验证预览和确认安装；HTTP mutation 必须单独审计。
5. [ ] 最后评估签名包、团队 Catalog 与 Marketplace；远程自动安装不属于基础版本。

## 已验证基线

- v69 最终全仓普通/race 测试分别通过于 259.7 秒/275.3 秒；vet、零告警 staticcheck、module verify/tidy diff 与零可达漏洞 govulncheck 通过。
- parser 的既有 20 秒约 2645 万次 fuzz 基线保持有效；新增对象发布、Store、Application 和 CLI 定向测试覆盖三轮 race、双 Store 收敛、崩溃恢复、腐坏/symlink/取消、伪造收据和 SQL 旁路；真实双 Service 独立生成候选身份的导入/移除收敛另通过 20 轮普通与 10 轮 race。首次 Linux CI run `29556933994` 暴露并发嵌套目录准备失败；逐级创建并验证真实目录后，12 个独立 Store 通过 100 轮普通与 20 轮 race。
- TypeScript/OpenAPI/production build、9 个文件 21 项前端测试、零漏洞 npm audit 通过；v69 未改变 HTTP 契约。
- 审计修复旧 schema 夹具未先移除 v69 trigger、错误文本静态规则、冗余临时清理状态、对象收据接口绑定、发布前取消点、并发请求因独立 ID/时间被误判为改意图，以及 Manifest 自由文本 description 未脱敏进入 SQLite；当前无已知未解决高/中风险。
- 首次 Linux CI run `29556933994` 暴露并发嵌套目录准备失败；修复提交 `d28b100` 的 GitHub Actions run `29557803407` 已全绿（Go/Linux 3 分 21 秒，TypeScript 23 秒）。

## 验收标准

- 恶意 ZIP、路径逃逸、链接、特殊文件、超大包、重复项、Unicode/大小写碰撞和内容 hash 漂移全部失败关闭。
- 导入前后不会执行命令、访问网络、调用模型或改变工具权限。
- 并发导入、重启恢复和相同 operation 重放收敛到同一不可变版本。
- v69 已保证安装与 tombstone 跨重启恢复，并为已固定版本提供 Go/SQL 移除门禁；v70 仍需完成外部 Run 固定版本与上下文恢复验收。
- CLI 与 Desktop 对同一包产生相同 digest、风险预览、错误码和安装结果。
