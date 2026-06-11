# deepx-code 演示录制脚本(vhs)—— 工作模式切换
#
# 用法:
#   brew install vhs          # macOS;其它平台见 https://github.com/charmbracelet/vhs
#   vhs scripts/demo.tape     # 在仓库根目录跑,产出 assets/demo.gif
#
# 前置:`deepx` 需在 PATH 且为最新构建(go install / make install),并已配置好
#       ~/.deepx/model.yaml(演示本身不发模型调用,但启动要读配置,否则会弹首启向导)。
# 说明:本演示是纯 UI(/working-mode 弹窗选择),不消耗 token、时长确定,无需按网络微调。

Output assets/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1200
Set Height 1000
Set Padding 18
Set Theme "Catppuccin Mocha"

# 在一个干净空目录里启动,并清掉它的历史会话 → 每次都从空白界面开始(预热不录)。
# 会话存于 ~/.deepx/sessions/<sha1(workspace)[:16]>,这里按同一算法定位并删除。
Hide
Type "mkdir -p /tmp/deepx-demo; rm -rf ~/.deepx/sessions/$(printf %s /tmp/deepx-demo | shasum | cut -c1-16); clear; cd /tmp/deepx-demo && deepx"
Enter
Sleep 5s
Show

# 展示完整初始界面:左 chat 区 + 右状态面板(banner / 工作区 / 模型 / 工作模式 / 沙箱 全可见)
Sleep 2.2s

# 打开工作模式选择弹窗
Type "/working-mode"
Sleep 900ms
Enter
Sleep 1.6s

# 在三种模式间切换:karpathy → openspec → superpowers → 回 openspec
Down
Sleep 1.1s
Down
Sleep 1.1s
Up
Sleep 1.1s

# 应用 openspec —— 右栏「工作模式」与提示同步更新
Enter
Sleep 3.2s
