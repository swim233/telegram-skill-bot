package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/ijnkawakaze/telegram-bot-api"
	"github.com/spf13/viper"
	"github.com/swim233/chat_bot/api"
	"github.com/swim233/chat_bot/bot"
	"github.com/swim233/chat_bot/config"
	"github.com/swim233/chat_bot/data"
	apiConfig "github.com/swim233/chat_bot/internal/bot/api"
	task "github.com/swim233/chat_bot/internal/bot/task"
	"github.com/swim233/chat_bot/utils"
	"github.com/swim233/logger"
)

func main() {
	logger.SkipCaller = 1
	logger.InitLogger()
	config.InitViper()
	config.LoadPermissions()
	err := data.InitDB()
	if err != nil {
		logger.Panic("fail to init database: %s", err.Error())
	}
	task.TaskManagerInstance.InitTaskManager()
	err = api.InitRestyClient()
	if err != nil {
		logger.Panic("fail to init resty client: %s", err.Error())
	}
	bot.InitBot()
	b := bot.Bot.AddHandle()
	b.NewProcessor(func(update tgbotapi.Update) bool {
		return allowUpdate(update) && update.Message != nil && update.Message.Chat != nil && !update.Message.Chat.IsPrivate() && !update.Message.IsCommand() && (update.Message.From == nil || !update.Message.From.IsBot)
	}, func(update tgbotapi.Update) error {
		return data.SaveGroupMessage(update)
	})
	b.NewCommandProcessor("del", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		err := task.TaskManagerInstance.AddDelayTask(update, "del")
		if err != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return err
		}
		return nil
	}))
	b.NewCommandProcessor("reply", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		err := task.TaskManagerInstance.AddDelayTask(update, "reply")
		if err != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return err
		}
		return nil
	}))
	b.NewCommandProcessor("cancel", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		subcommand := strings.TrimSpace(update.Message.CommandArguments())
		result := task.TaskManagerInstance.CancelTask(update, subcommand)
		if result != "" {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, result)
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
		}
		return nil
	}))
	b.NewCommandProcessor("skill", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		return handleSkill(update)
	}))
	b.NewProcessor(func(update tgbotapi.Update) bool {
		return allowUpdate(update) && update.Message != nil && !update.Message.IsCommand() && isCaptionCommand(update.Message.Caption, "skill")
	}, asyncHandler(func(update tgbotapi.Update) error {
		_ = data.SaveGroupMessage(update)
		return handleSkill(update)
	}))
	b.NewCommandProcessor("switch", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		rsp, err := apiConfig.SwitchAction(update)
		if err != nil {
			logger.Error("Error in switching module: %s", err.Error())
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, utils.FoldText2Html("切换配置时出错：", err.Error()))
			msg.ParseMode = tgbotapi.ModeHTML
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return err
		}
		bot.Bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, rsp))
		return nil
	}))
	b.NewCommandProcessor("list", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		if update.Message == nil || update.Message.Chat == nil {
			return nil
		}
		args := strings.Fields(update.Message.CommandArguments())
		if len(args) != 1 {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "用法: /list skill | /list summary | /list api | /list perm | /list command")
			msg.ReplyToMessageID = update.Message.MessageID
			_, err := bot.Bot.Send(msg)
			return err
		}

		switch strings.ToLower(args[0]) {
		case "skill", "summary":
			models, err := api.GetModelsByScene(strings.ToLower(args[0]))
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, utils.FoldText2Html("列出模型失败", err.Error()))
				msg.ParseMode = tgbotapi.ModeHTML
				msg.ReplyToMessageID = update.Message.MessageID
				_, _ = bot.Bot.Send(msg)
				return err
			}
			if len(models) == 0 {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "当前没有可用模型")
				msg.ReplyToMessageID = update.Message.MessageID
				_, err := bot.Bot.Send(msg)
				return err
			}
			return sendLongHtmlFoldMessage(
				update.Message.Chat.ID,
				update.Message.MessageID,
				strings.ToUpper(args[0])+" 模型列表",
				strings.Join(models, "\n"),
			)
		case "command", "commands":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "该子命令已迁移，请使用 /help")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		case "api":
			if !config.IsCommandAllowed("list_api") && !task.CheckBotOwner(update) && !config.HasPermission(update.Message.Chat.ID, update.Message.From.ID, "list_api") {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "无权查看 API 列表，请联系 owner 授权")
				msg.ReplyToMessageID = update.Message.MessageID
				_, err := bot.Bot.Send(msg)
				return err
			}
			out, err := apiConfig.ListAPIWithMask()
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, utils.FoldText2Html("列出 API 失败", err.Error()))
				msg.ParseMode = tgbotapi.ModeHTML
				msg.ReplyToMessageID = update.Message.MessageID
				_, _ = bot.Bot.Send(msg)
				return err
			}
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, out)
			msg.ReplyToMessageID = update.Message.MessageID
			_, err = bot.Bot.Send(msg)
			return err
		case "perm", "perms", "approve":
			if !task.CheckBotOwner(update) {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "仅 BOT owner 可查看权限列表")
				msg.ReplyToMessageID = update.Message.MessageID
				bot.Bot.Send(msg)
				return nil
			}
			perms := config.ListChatPermissions(update.Message.Chat.ID)
			if len(perms) == 0 {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "当前群组无授权记录")
				msg.ReplyToMessageID = update.Message.MessageID
				bot.Bot.Send(msg)
				return nil
			}
			var sb strings.Builder
			sb.WriteString("当前群组授权列表:\n")
			for uid, cmds := range perms {
				fmt.Fprintf(&sb, "\n用户 %d:\n  %s\n", uid, strings.Join(cmds, ", "))
			}
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, sb.String())
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		default:
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "未知子命令: "+args[0]+"\n用法: /list skill | /list summary | /list api | /list perm | /list command")
			msg.ReplyToMessageID = update.Message.MessageID
			_, err := bot.Bot.Send(msg)
			return err
		}
	}))
	b.NewCommandProcessor("summary", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		if update.Message == nil || update.Message.Chat == nil {
			return nil
		}
		chatID := update.Message.Chat.ID
		userID := update.Message.From.ID
		if !config.IsCommandAllowed("summary") && !task.CheckBotOwner(update) && !config.HasPermission(chatID, userID, "summary") {
			msg := tgbotapi.NewMessage(chatID, "无权使用 /summary，请联系 owner 授权")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		}
		start, end, parseErr := parseSummaryRange(update.Message.CommandArguments())
		if parseErr != nil {
			msg := tgbotapi.NewMessage(chatID, "时间参数格式错误，示例: /summary 24h、/summary 30m、/summary 1d 或 /summary 26.6.7 8h0m-20h0m")
			msg.ReplyToMessageID = update.Message.MessageID
			_, sendErr := bot.Bot.Send(msg)
			if sendErr != nil {
				return sendErr
			}
			return parseErr
		}
		prompt, err := data.BuildSummaryPromptByRange(chatID, start, end)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, utils.FoldText2Html("构建总结数据失败", err.Error()))
			msg.ParseMode = tgbotapi.ModeHTML
			msg.ReplyToMessageID = update.Message.MessageID
			_, _ = bot.Bot.Send(msg)
			return err
		}
		if prompt != "" {
			logSummaryPromptPreview(prompt, 20)
		}
		if prompt == "" {
			msg := tgbotapi.NewMessage(chatID, "指定时间范围内暂无可总结消息。")
			msg.ReplyToMessageID = update.Message.MessageID
			_, err := bot.Bot.Send(msg)
			return err
		}
		// 先发占位消息
		pending := tgbotapi.NewMessage(chatID, "⏳ 等待响应中...")
		pending.ReplyToMessageID = update.Message.MessageID
		sent, sendErr := bot.Bot.Send(pending)
		if sendErr != nil {
			return sendErr
		}
		pendingMsgID := sent.MessageID

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		summaryKey := task.MessageKey{ChatID: chatID, MessageID: update.Message.MessageID}
		task.TaskManagerInstance.RegisterAPIContext(summaryKey, cancel)
		defer task.TaskManagerInstance.UnregisterAPIContext(summaryKey, cancel)
		rsp, err := api.SendRequestByScene(ctx, prompt, "summary")
		if err != nil {
			if ctx.Err() != nil {
				editMsg := tgbotapi.NewEditMessageText(chatID, pendingMsgID, "summary 请求已取消")
				bot.Bot.Send(editMsg)
				return nil
			}
			errText := utils.FoldText2Html("AI 总结失败", err.Error())
			editMsg := tgbotapi.NewEditMessageText(chatID, pendingMsgID, errText)
			editMsg.ParseMode = tgbotapi.ModeHTML
			bot.Bot.Send(editMsg)
			return err
		}
		return editOrSendMarkdownAsFoldedHTML(chatID, pendingMsgID, update.Message.MessageID, "群聊日报总结", rsp)
	}))
	b.NewCommandProcessor("focus", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		if update.Message == nil || update.Message.Chat == nil {
			return nil
		}
		chatID := update.Message.Chat.ID
		userID := update.Message.From.ID
		if !config.IsCommandAllowed("focus") && !task.CheckBotOwner(update) && !config.HasPermission(chatID, userID, "focus") {
			msg := tgbotapi.NewMessage(chatID, "无权使用 /focus，请联系 owner 授权")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		}
		result := handleFocus(update)
		return result
	}))
	b.NewCommandProcessor("approve", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		if !task.CheckBotOwner(update) {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "仅 BOT owner 可执行 /approve")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		}
		result := handleApprove(update)
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, result)
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return nil
	}))
	b.NewCommandProcessor("revoke", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		if !task.CheckBotOwner(update) {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "仅 BOT owner 可执行 /revoke")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		}
		result := handleRevoke(update)
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, result)
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return nil
	}))
	b.NewCommandProcessor("help", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		commands := []string{
			"/del <duration> — 定时删除消息",
			"/reply <duration> <content> — 定时回复消息",
			"/cancel [ai|task] — 取消任务（回复目标消息）",
			"/summary [duration | date timeRange] — AI 群聊总结，例: /summary 24h、/summary 1d、/summary 26.6.7 8h0m-20h0m",
			"/skill <prompt> — AI 问答",
			"/focus <duration|date|条数> <content> — 聚焦分析聊天记录",
			"/switch models <skill|summary> <model_id> — 切换模型",
			"/switch api <skill|summary> <api_name> [token_index] — 切换 API",
			"/approve <command> [user_id] — 授权用户使用命令",
			"/revoke [command] [user_id] — 撤销授权",
			"/list skill|summary|api|perm — 查看配置",
			"/help — 显示此帮助",
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "可用命令:\n"+strings.Join(commands, "\n"))
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return nil
	}))
	b.NewCommandProcessor("test", asyncHandler(func(update tgbotapi.Update) error {
		if !allowUpdate(update) {
			return nil
		}
		_ = data.SaveGroupMessage(update)
		fmtText := utils.FoldText2Html("测试标题", "你好，下面是一段用于你测试的纯文本示例：本句不包含任何需要转义的符号，仅用普通中文和常见标点来组成内容。你可以在 Go 里直接把它当作字符串内容验证读取、长度统计与编码处理是否正常。请留意其中不含尖括号、反斜杠、引号等可能触发转义的字符；也不出现换行。继续检查即可。")
		logger.Debug(fmtText)
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmtText)
		msg.ParseMode = tgbotapi.ModeHTML
		_, err := bot.Bot.Send(msg)
		return err
	}))

	b.Run()
}

func asyncHandler(fn func(tgbotapi.Update) error) func(tgbotapi.Update) error {
	return func(update tgbotapi.Update) error {
		go func() {
			if err := fn(update); err != nil {
				logger.Error("async handler error: %s", err.Error())
			}
		}()
		return nil
	}
}

func allowUpdate(update tgbotapi.Update) bool {
	if update.Message == nil {
		return true
	}
	if update.Message.From == nil || update.Message.Chat == nil {
		return true
	}
	if task.CheckBotOwner(update) {
		return true
	}
	if isAllowedChatID(update.Message.Chat.ID) {
		return true
	}
	if len(config.GetPermissions(update.Message.Chat.ID, update.Message.From.ID)) > 0 {
		return true
	}
	return false
}

func handleSkill(update tgbotapi.Update) error {
	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	if !config.IsCommandAllowed("skill") && !task.CheckBotOwner(update) && !config.HasPermission(chatID, userID, "skill") {
		msg := tgbotapi.NewMessage(chatID, "无权使用 /skill，请联系 owner 授权")
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return nil
	}

	// 先发占位消息
	pending := tgbotapi.NewMessage(chatID, "⏳ 等待响应中...")
	pending.ReplyToMessageID = update.Message.MessageID
	sent, err := bot.Bot.Send(pending)
	if err != nil {
		return err
	}
	pendingMsgID := sent.MessageID

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := task.MessageKey{ChatID: chatID, MessageID: update.Message.MessageID}
	task.TaskManagerInstance.RegisterAPIContext(key, cancel)
	defer task.TaskManagerInstance.UnregisterAPIContext(key, cancel)

	rsp, err := api.SendSkillRequest(ctx, update)
	if err != nil {
		if ctx.Err() != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, pendingMsgID, "skill 请求已取消")
			bot.Bot.Send(editMsg)
			return nil
		}
		errText := utils.FoldText2Html("请求api时发生错误 当前模型: "+viper.GetString("API.skill_module"), err.Error())
		editMsg := tgbotapi.NewEditMessageText(chatID, pendingMsgID, errText)
		editMsg.ParseMode = tgbotapi.ModeHTML
		bot.Bot.Send(editMsg)
		return err
	}
	rsp = stripThinkingBlock(rsp)
	if err := editOrSendMarkdownAsFoldedHTML(chatID, pendingMsgID, update.Message.MessageID, "AI回复", rsp); err != nil {
		logger.Error("send skill response failed: %s", err.Error())
		return sendLongPlainTextMessage(chatID, update.Message.MessageID, rsp)
	}
	return nil
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "%", "%%")
	return s
}

var (
	reTelegramLink = regexp.MustCompile(`\[([^\]]*)\]\(https://t\.me/c/(\d+)/(\d+)\)`)
	reDateArg      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

func validateTelegramLinks(response string, expectedChatID string) string {
	return reTelegramLink.ReplaceAllStringFunc(response, func(match string) string {
		parts := reTelegramLink.FindStringSubmatch(match)
		if parts == nil || parts[2] != expectedChatID {
			return ""
		}
		return match
	})
}

// handleFocus 处理 /focus 命令
func handleFocus(update tgbotapi.Update) error {
	chatID := update.Message.Chat.ID
	args := strings.Fields(update.Message.CommandArguments())

	if len(args) < 2 {
		msg := tgbotapi.NewMessage(chatID, "用法:\n/focus <duration> <content>\n/focus <date> <duration> <content>\n/focus <数字条数> <content>\n\n示例:\n/focus 12h 总结关于技术的讨论\n/focus 2026-04-25 6h 找关于部署的内容\n/focus 500 最近讨论了什么")
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return nil
	}

	var messages string
	var count int
	var err error
	var content string

	// 解析参数
	if reDateArg.MatchString(args[0]) {
		// /focus <date> <duration> <content>
		if len(args) < 3 {
			msg := tgbotapi.NewMessage(chatID, "指定日期时需要同时提供 duration 和 content\n用法: /focus 2026-04-25 12h 内容")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		}
		startDate, parseErr := time.Parse("2006-01-02", args[0])
		if parseErr != nil {
			msg := tgbotapi.NewMessage(chatID, "日期格式错误，应为 yyyy-mm-dd")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		}
		duration, parseErr := time.ParseDuration(args[1])
		if parseErr != nil {
			msg := tgbotapi.NewMessage(chatID, "时间段格式错误，示例: 12h, 30m, 1d（注意: 天用 24h 表示）")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Bot.Send(msg)
			return nil
		}
		endTime := startDate.Add(duration)
		content = strings.Join(args[2:], " ")
		messages, count, err = data.QueryMessagesByTimeRange(chatID, startDate, endTime)
	} else if num, parseErr := strconv.Atoi(args[0]); parseErr == nil && num > 0 {
		// /focus <capacity> <content>
		content = strings.Join(args[1:], " ")
		messages, count, err = data.QueryMessagesByCapacity(chatID, num)
	} else if _, parseErr := time.ParseDuration(args[0]); parseErr == nil {
		// /focus <duration> <content>
		duration, _ := time.ParseDuration(args[0])
		endTime := time.Now().UTC()
		startTime := endTime.Add(-duration)
		content = strings.Join(args[1:], " ")
		messages, count, err = data.QueryMessagesByTimeRange(chatID, startTime, endTime)
	} else {
		msg := tgbotapi.NewMessage(chatID, "无法识别第一个参数，应为日期(yyyy-mm-dd)、时间段(如12h)或消息条数(如500)")
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return nil
	}

	if err != nil {
		msg := tgbotapi.NewMessage(chatID, utils.FoldText2Html("查询消息失败", err.Error()))
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return err
	}
	if count == 0 {
		msg := tgbotapi.NewMessage(chatID, "指定范围内暂无消息记录。")
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Bot.Send(msg)
		return nil
	}

	// 发送 pending 消息
	pending := tgbotapi.NewMessage(chatID, fmt.Sprintf("⏳ 正在分析 %d 条消息...", count))
	pending.ReplyToMessageID = update.Message.MessageID
	sent, sendErr := bot.Bot.Send(pending)
	if sendErr != nil {
		return sendErr
	}
	pendingMsgID := sent.MessageID

	// 计算 ChatID（去负号，去 100 前缀）
	absChatID := chatID
	if absChatID < 0 {
		absChatID = -absChatID
	}
	// Telegram supergroup IDs (after negation) are 13 digits starting with "100";
	// t.me/c/ links need the "100" prefix stripped.
	chatIDStr := fmt.Sprintf("%d", absChatID)
	if len(chatIDStr) >= 13 && strings.HasPrefix(chatIDStr, "100") {
		chatIDStr = chatIDStr[3:]
	}

	userFullName := update.Message.From.FullName()

	// 构建 prompt
	prompt := fmt.Sprintf(data.FocusPrompt, chatIDStr, userFullName, escapeXML(content), escapeXML(messages))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := task.MessageKey{ChatID: chatID, MessageID: update.Message.MessageID}
	task.TaskManagerInstance.RegisterAPIContext(key, cancel)
	defer task.TaskManagerInstance.UnregisterAPIContext(key, cancel)

	rsp, err := api.SendRequestByScene(ctx, prompt, "focus")
	if err != nil {
		if ctx.Err() != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, pendingMsgID, "focus 请求已取消")
			bot.Bot.Send(editMsg)
			return nil
		}
		errText := utils.FoldText2Html("focus 请求失败", err.Error())
		editMsg := tgbotapi.NewEditMessageText(chatID, pendingMsgID, errText)
		editMsg.ParseMode = tgbotapi.ModeHTML
		bot.Bot.Send(editMsg)
		return err
	}
	rsp = stripThinkingBlock(rsp)
	rsp = validateTelegramLinks(rsp, chatIDStr)
	return editOrSendMarkdownAsFoldedHTML(chatID, pendingMsgID, update.Message.MessageID, "Focus 分析结果", rsp)
}

// handleApprove 处理 /approve <command> [user_id] 或回复消息
func handleApprove(update tgbotapi.Update) string {
	chatID := update.Message.Chat.ID
	args := strings.Fields(update.Message.CommandArguments())

	if len(args) == 0 {
		return "用法: /approve <command|all> [user_id]\n可用命令: all, " + strings.Join(config.ValidCommands, ", ")
	}

	command := strings.ToLower(args[0])
	if command != "all" && !config.IsValidCommand(command) {
		return "未知命令: " + command + "\n可用命令: all, " + strings.Join(config.ValidCommands, ", ")
	}

	var userID int64
	if len(args) >= 2 {
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return "无效的用户 ID: " + args[1]
		}
		userID = id
	} else if update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.From != nil {
		userID = update.Message.ReplyToMessage.From.ID
	} else {
		return "请指定用户 ID 或回复一条消息"
	}

	if command == "all" {
		for _, cmd := range config.ValidCommands {
			if err := config.GrantPermission(chatID, userID, cmd); err != nil {
				return "授权失败: " + err.Error()
			}
		}
		return fmt.Sprintf("已授权用户 %d 在群组 %d 使用所有命令", userID, chatID)
	}

	if err := config.GrantPermission(chatID, userID, command); err != nil {
		return "授权失败: " + err.Error()
	}
	return fmt.Sprintf("已授权用户 %d 在群组 %d 使用 /%s", userID, chatID, command)
}

// handleRevoke 处理 /revoke [command] [user_id] 或回复消息
func handleRevoke(update tgbotapi.Update) string {
	chatID := update.Message.Chat.ID
	args := strings.Fields(update.Message.CommandArguments())

	// /revoke（回复消息）— 撤销被回复者所有权限
	if len(args) == 0 {
		if update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.From != nil {
			userID := update.Message.ReplyToMessage.From.ID
			if err := config.RevokeAllPermissions(chatID, userID); err != nil {
				return "撤销失败: " + err.Error()
			}
			return fmt.Sprintf("已撤销用户 %d 在群组 %d 的所有权限", userID, chatID)
		}
		return "用法: /revoke [command] <user_id> 或回复一条消息"
	}

	// /revoke <user_id> — 纯数字，撤销该用户所有权限
	if len(args) == 1 {
		if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
			if err := config.RevokeAllPermissions(chatID, id); err != nil {
				return "撤销失败: " + err.Error()
			}
			return fmt.Sprintf("已撤销用户 %d 在群组 %d 的所有权限", id, chatID)
		}
		// /revoke <command>（回复消息）
		command := strings.ToLower(args[0])
		if !config.IsValidCommand(command) {
			return "未知命令: " + command + "\n可用命令: " + strings.Join(config.ValidCommands, ", ")
		}
		if update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.From != nil {
			userID := update.Message.ReplyToMessage.From.ID
			if err := config.RevokePermission(chatID, userID, command); err != nil {
				return "撤销失败: " + err.Error()
			}
			return fmt.Sprintf("已撤销用户 %d 的 /%s 权限", userID, command)
		}
		return "请指定用户 ID 或回复一条消息"
	}

	// /revoke <command> <user_id>
	command := strings.ToLower(args[0])
	if !config.IsValidCommand(command) {
		return "未知命令: " + command + "\n可用命令: " + strings.Join(config.ValidCommands, ", ")
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "无效的用户 ID: " + args[1]
	}
	if err := config.RevokePermission(chatID, id, command); err != nil {
		return "撤销失败: " + err.Error()
	}
	return fmt.Sprintf("已撤销用户 %d 的 /%s 权限", id, command)
}

func isCaptionCommand(caption, command string) bool {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return false
	}
	first := strings.Fields(caption)[0]
	first = strings.TrimPrefix(first, "/")
	if idx := strings.Index(first, "@"); idx >= 0 {
		first = first[:idx]
	}
	return strings.EqualFold(first, command)
}

func isAllowedChatID(chatID int64) bool {
	allowList := getAllowList()
	if len(allowList) == 0 {
		return true
	}
	for _, id := range allowList {
		if id == 0 {
			return true
		}
		if id == chatID {
			return true
		}
	}
	return false
}

func getAllowList() []int64 {
	var list []int64
	if err := viper.UnmarshalKey("BOT.allow_list", &list); err != nil {
		return nil
	}
	return list
}

func sendLongHtmlFoldMessage(chatID int64, replyTo int, title, content string) error {
	const maxLen = 3200
	runes := []rune(content)
	for start := 0; start < len(runes); start += maxLen {
		end := start + maxLen
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[start:end])
		text := utils.FoldText2Html(title, chunk)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyToMessageID = replyTo
		_, err := bot.Bot.Send(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func sendLongPlainTextMessage(chatID int64, replyTo int, content string) error {
	const maxLen = 4000
	runes := []rune(content)
	for start := 0; start < len(runes); start += maxLen {
		end := start + maxLen
		if end > len(runes) {
			end = len(runes)
		}
		msg := tgbotapi.NewMessage(chatID, string(runes[start:end]))
		msg.ReplyToMessageID = replyTo
		_, err := bot.Bot.Send(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

// editOrSendMarkdownAsFoldedHTML 尝试编辑占位消息为结果；超长时删除占位并分片发送新消息
func editOrSendMarkdownAsFoldedHTML(chatID int64, pendingMsgID int, replyTo int, title, mdContent string) error {
	fullHTML := utils.MarkdownToFoldedHTML(title, mdContent)
	if len([]rune(fullHTML)) <= 4000 {
		editMsg := tgbotapi.NewEditMessageText(chatID, pendingMsgID, fullHTML)
		editMsg.ParseMode = tgbotapi.ModeHTML
		_, err := bot.Bot.Send(editMsg)
		if err != nil {
			// 编辑失败时 fallback：删除占位，发新消息
			bot.Bot.Send(tgbotapi.NewDeleteMessage(chatID, pendingMsgID))
			return sendLongMarkdownAsFoldedHTMLMessage(chatID, replyTo, title, mdContent)
		}
		return nil
	}
	// 超长：删除占位消息，分片发送
	bot.Bot.Send(tgbotapi.NewDeleteMessage(chatID, pendingMsgID))
	return sendLongMarkdownAsFoldedHTMLMessage(chatID, replyTo, title, mdContent)
}

func sendLongMarkdownAsFoldedHTMLMessage(chatID int64, replyTo int, title, mdContent string) error {
	fullHTML := utils.MarkdownToFoldedHTML(title, mdContent)
	// Telegram 单条消息上限约 4096 字符
	if len([]rune(fullHTML)) <= 4000 {
		msg := tgbotapi.NewMessage(chatID, fullHTML)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyToMessageID = replyTo
		_, err := bot.Bot.Send(msg)
		return err
	}
	// 超长内容：按原文分片，每片单独转换
	const maxLen = 3000
	runes := []rune(mdContent)
	for start := 0; start < len(runes); start += maxLen {
		end := start + maxLen
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[start:end])
		text := utils.MarkdownToFoldedHTML(title, chunk)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyToMessageID = replyTo
		_, err := bot.Bot.Send(msg)
		if err != nil {
			// HTML 解析失败时 fallback 纯文本
			msg = tgbotapi.NewMessage(chatID, title+"\n\n"+chunk)
			msg.ReplyToMessageID = replyTo
			_, err = bot.Bot.Send(msg)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func stripThinkingBlock(s string) string {
	for {
		startTag := "<think>"
		endTag := "</think>"
		startIdx := strings.Index(s, startTag)
		if startIdx < 0 {
			break
		}
		endIdx := strings.Index(s[startIdx:], endTag)
		if endIdx < 0 {
			s = s[:startIdx]
			break
		}
		s = s[:startIdx] + s[startIdx+endIdx+len(endTag):]
	}
	return strings.TrimSpace(s)
}

// summaryTZ 是 /summary 时间参数采用的固定时区（UTC+8）。
var summaryTZ = time.FixedZone("UTC+8", 8*3600)

var (
	summaryDurationRe = regexp.MustCompile(`^(?:\d+d)?(?:\d+h)?(?:\d+m)?$`)
	summaryDateRe     = regexp.MustCompile(`^(\d{2}|\d{4})\.(\d{1,2})\.(\d{1,2})$`)
	summaryClockRe    = regexp.MustCompile(`^(\d{1,2})h(?:(\d{1,2})m)?$`)
)

// parseSummaryRange 解析 /summary 的时间参数，返回 UTC 起止时间。
// 支持两种形式：
//  1. 相对时长（m/h/d 任意组合）："30m"、"24h"、"1d12h30m"；空串默认 1d。
//  2. 指定日期 + 当天时段（按 UTC+8 解释）："26.6.7 8h0m-20h0m"。
func parseSummaryRange(arg string) (time.Time, time.Time, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		end := time.Now().UTC()
		return end.Add(-24 * time.Hour), end, nil
	}

	fields := strings.Fields(arg)
	switch len(fields) {
	case 1:
		d, err := parseMHDDuration(fields[0])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end := time.Now().UTC()
		return end.Add(-d), end, nil
	case 2:
		date, err := parseSummaryDate(fields[0])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		startClock, endClock, err := parseSummaryClockRange(fields[1])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		start := time.Date(date.Year(), date.Month(), date.Day(),
			startClock[0], startClock[1], 0, 0, summaryTZ)
		end := time.Date(date.Year(), date.Month(), date.Day(),
			endClock[0], endClock[1], 0, 0, summaryTZ)
		if !end.After(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("结束时间必须晚于起始时间")
		}
		return start.UTC(), end.UTC(), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("参数数量不正确，应为 1 或 2 个")
	}
}

func parseMHDDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || !summaryDurationRe.MatchString(s) {
		return 0, fmt.Errorf("时长格式错误: %q，仅支持 m/h/d，例如 30m、24h、1d、1d12h", s)
	}
	var total time.Duration
	var hasUnit bool
	rest := s
	for _, unit := range []struct {
		suffix string
		mul    time.Duration
	}{
		{"d", 24 * time.Hour},
		{"h", time.Hour},
		{"m", time.Minute},
	} {
		idx := strings.Index(rest, unit.suffix)
		if idx < 0 {
			continue
		}
		n, err := strconv.Atoi(rest[:idx])
		if err != nil {
			return 0, fmt.Errorf("时长格式错误: %q", s)
		}
		total += time.Duration(n) * unit.mul
		rest = rest[idx+len(unit.suffix):]
		hasUnit = true
	}
	if !hasUnit || rest != "" || total <= 0 {
		return 0, fmt.Errorf("时长格式错误: %q", s)
	}
	return total, nil
}

func parseSummaryDate(s string) (time.Time, error) {
	m := summaryDateRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("日期格式错误: %q，应为 YY.M.D 或 YYYY.M.D", s)
	}
	year, _ := strconv.Atoi(m[1])
	if len(m[1]) == 2 {
		year += 2000
	}
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("日期数值非法: %q", s)
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, summaryTZ)
	// time.Date 会静默规范化非法日期（如 2.30 -> 3.2），需回查原值
	if t.Year() != year || t.Month() != time.Month(month) || t.Day() != day {
		return time.Time{}, fmt.Errorf("日期数值非法: %q", s)
	}
	return t, nil
}

func parseSummaryClockRange(s string) ([2]int, [2]int, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return [2]int{}, [2]int{}, fmt.Errorf("时段格式错误: %q，应为 Xh[Ym]-Xh[Ym]", s)
	}
	start, err := parseSummaryClock(parts[0])
	if err != nil {
		return [2]int{}, [2]int{}, err
	}
	end, err := parseSummaryClock(parts[1])
	if err != nil {
		return [2]int{}, [2]int{}, err
	}
	return start, end, nil
}

func parseSummaryClock(s string) ([2]int, error) {
	m := summaryClockRe.FindStringSubmatch(s)
	if m == nil {
		return [2]int{}, fmt.Errorf("时刻格式错误: %q，应为 Xh 或 XhYm", s)
	}
	hour, _ := strconv.Atoi(m[1])
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return [2]int{}, fmt.Errorf("时刻数值非法: %q", s)
	}
	return [2]int{hour, minute}, nil
}

func logSummaryPromptPreview(prompt string, lines int) {
	parts := strings.SplitN(prompt, "\n", lines+1)
	if len(parts) > lines {
		parts = parts[:lines]
	}
	logger.Info("summary prompt preview (%d lines):\n%s", len(parts), strings.Join(parts, "\n"))
}
