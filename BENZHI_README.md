# BENZHI_README

## 项目说明
- 项目：benzhi-project-3ecdb744-fcbd-4d20-a5c1-66c41953d97c
- 项目用途：已完整实现公共应急避难场所演练就绪认证工作台，覆盖版本化基线、启用证据、顺序现场记录、确定性问题判定、整改复测、不可变送审快照、职责分离复核、SHA-256 审计链和只读就绪档案验证。
- Go 工具链：`golang:1.23`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：shelter-drill-gate
- 项目介绍：面向公共应急避难场所管理团队的演练就绪认证工作台，将演练方案冻结、现场检查点记录、缺陷整改复测、独立复核和可验证就绪档案收束为一条有明确状态迁移的业务流程。
- 项目概述：面向公共应急避难场所管理团队的演练就绪认证工作台，将演练方案冻结、现场检查点记录、缺陷整改复测、独立复核和可验证就绪档案收束为一条有明确状态迁移的业务流程。
- 核心工作流：协调员创建演练档案并冻结场景、岗位、检查点和判定阈值，完成启用证据登记后启动演练；现场记录按时间顺序提交并由规则即时形成待整改项，协调员逐项提交措施和定向复测结果，全部关闭后交由独立复核员审阅不可变快照，退回时回到整改复测状态，批准时生成带摘要的只读就绪档案并结束流程。
- 对外接口：Go 服务直接提供无需 Node 构建链的原生 HTML、CSS 和 JavaScript 单页工作台及同源 JSON 端点；页面包含演练状态轨迹、基线配置、现场记录表、问题整改队列、复核面板和终态档案校验视图，可在浏览器内完成整条流程。服务监听支持 -addr=127.0.0.1:<port>，同时在未传 -addr 时读取 PORT 并绑定 127.0.0.1:<PORT>，默认地址为 127.0.0.1:19081，绝不默认绑定 0.0.0.0 或常见低位端口。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/shelterd -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-3ecdb744-fcbd-4d20-a5c1-66c41953d97c-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-3ecdb744-fcbd-4d20-a5c1-66c41953d97c-arm64 linux/arm64

docker run -it benzhi-project-3ecdb744-fcbd-4d20-a5c1-66c41953d97c-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/shelterd -self-check -addr=127.0.0.1:19081`
