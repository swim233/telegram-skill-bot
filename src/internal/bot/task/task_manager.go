package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	tgbotapi "github.com/ijnkawakaze/telegram-bot-api"
	"github.com/spf13/viper"
	"github.com/swim233/chat_bot/bot"
	"github.com/swim233/logger"
)

var TaskManagerInstance TaskManager

const (
	DelayDeleteTask TaskType = "delay_del"
	DelayReplyTask  TaskType = "delay_reply"
)
const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusCancel  TaskStatus = "cancelled"
)

type TaskType string
type TaskStatus string

type MessageKey struct {
	ChatID    int64
	MessageID int
}

type Task struct {
	Update     tgbotapi.Update
	TaskID     uuid.UUID
	TaskType   TaskType
	TaskStatus TaskStatus
	TaskCtx    context.Context
	TaskCancel context.CancelFunc
	Delay      time.Duration
	ReplyText  string
	Keys       []MessageKey
}

type TaskManager struct {
	mu         sync.RWMutex
	tasks      map[MessageKey][]*Task
	taskQueue  chan *Task
	apiMu      sync.Mutex
	apiCancels map[MessageKey][]context.CancelFunc
}

func (t *TaskManager) InitTaskManager() {
	t.tasks = make(map[MessageKey][]*Task)
	t.taskQueue = make(chan *Task, 128)
	t.apiCancels = make(map[MessageKey][]context.CancelFunc)
	go t.consumeTasks()
}

func (t *TaskManager) consumeTasks() {
	for task := range t.taskQueue {
		go t.executeTask(task)
	}
}

func (t *TaskManager) addTask(task *Task, keys []MessageKey) {
	task.Keys = keys
	t.mu.Lock()
	for _, key := range keys {
		t.tasks[key] = append(t.tasks[key], task)
	}
	t.mu.Unlock()
}

func (t *TaskManager) removeTask(task *Task) {
	t.mu.Lock()
	for _, key := range task.Keys {
		tasks := t.tasks[key]
		for i, tk := range tasks {
			if tk.TaskID == task.TaskID {
				t.tasks[key] = append(tasks[:i], tasks[i+1:]...)
				break
			}
		}
		if len(t.tasks[key]) == 0 {
			delete(t.tasks, key)
		}
	}
	t.mu.Unlock()
}

func (t *TaskManager) enqueueTask(task *Task) {
	t.taskQueue <- task
}

func (t *TaskManager) AddDelayTask(u tgbotapi.Update, taskTypeStr string) error {
	if u.Message == nil || u.Message.Chat == nil {
		return errors.New("invalid message")
	}

	arg := strings.TrimSpace(u.Message.CommandArguments())
	args := strings.Fields(arg)

	taskType := getTaskType(taskTypeStr)
	if taskType == "" {
		return fmt.Errorf("unknown task type: %s", taskTypeStr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !CheckPermission(taskType, u) {
		cancel()
		return fmt.Errorf("permission denied")
	}
	if len(args) == 0 {
		bot.Bot.Send(tgbotapi.NewMessage(u.Message.Chat.ID, "未指定参数"))
		cancel()
		return nil
	}

	chatID := u.Message.Chat.ID

	switch taskType {
	case DelayDeleteTask:
		if u.Message.ReplyToMessage == nil {
			cancel()
			return errors.New("请回复一条消息后再使用 /del")
		}
		if len(args) != 1 {
			cancel()
			return errors.New("参数数量有误，应为 1 个")
		}
		delayTime, err := time.ParseDuration(args[0])
		if err != nil {
			cancel()
			logger.Error("Error in parsing time: %s", err.Error())
			return fmt.Errorf("格式化时间时发生错误: %s", err.Error())
		}
		newTask := &Task{
			Update:     u,
			TaskID:     uuid.New(),
			TaskType:   taskType,
			TaskStatus: TaskStatusPending,
			TaskCtx:    ctx,
			TaskCancel: cancel,
			Delay:      delayTime,
		}
		keys := []MessageKey{
			{ChatID: chatID, MessageID: u.Message.MessageID},
			{ChatID: chatID, MessageID: u.Message.ReplyToMessage.MessageID},
		}
		t.addTask(newTask, keys)
		t.enqueueTask(newTask)
		delMsg := tgbotapi.NewDeleteMessage(chatID, u.Message.MessageID)
		bot.Bot.Send(delMsg)
	case DelayReplyTask:
		if u.Message.ReplyToMessage == nil {
			cancel()
			return errors.New("请回复一条消息后再使用 /reply")
		}
		if len(args) < 2 {
			cancel()
			return errors.New("参数数量有误，用法: /reply <duration> <content>")
		}
		delayTime, err := time.ParseDuration(args[0])
		if err != nil {
			cancel()
			logger.Error("Error in parsing time: %s", err.Error())
			return fmt.Errorf("格式化时间时发生错误: %s", err.Error())
		}
		replyText := strings.TrimSpace(strings.TrimPrefix(arg, args[0]))
		if replyText == "" {
			cancel()
			return errors.New("回复内容不能为空")
		}
		newTask := &Task{
			Update:     u,
			TaskID:     uuid.New(),
			TaskType:   taskType,
			TaskStatus: TaskStatusPending,
			TaskCtx:    ctx,
			TaskCancel: cancel,
			Delay:      delayTime,
			ReplyText:  replyText,
		}
		keys := []MessageKey{
			{ChatID: chatID, MessageID: u.Message.MessageID},
			{ChatID: chatID, MessageID: u.Message.ReplyToMessage.MessageID},
		}
		t.addTask(newTask, keys)
		t.enqueueTask(newTask)
		delMsg := tgbotapi.NewDeleteMessage(chatID, u.Message.MessageID)
		bot.Bot.Send(delMsg)
	default:
		cancel()
		return fmt.Errorf("unsupported task type: %s", taskTypeStr)
	}

	return nil
}

func (t *TaskManager) RegisterAPIContext(key MessageKey, cancel context.CancelFunc) {
	t.apiMu.Lock()
	t.apiCancels[key] = append(t.apiCancels[key], cancel)
	t.apiMu.Unlock()
}

func (t *TaskManager) UnregisterAPIContext(key MessageKey, cancel context.CancelFunc) {
}

func (t *TaskManager) CancelTask(u tgbotapi.Update, subcommand string) string {
	if u.Message == nil || u.Message.Chat == nil {
		return ""
	}
	if !CheckPermission("cancel", u) {
		return "权限不足"
	}
	if u.Message.ReplyToMessage == nil {
		return "请回复一条消息后使用 /cancel"
	}

	key := MessageKey{
		ChatID:    u.Message.Chat.ID,
		MessageID: u.Message.ReplyToMessage.MessageID,
	}

	var cancelledTasks, cancelledAPI int

	if subcommand == "" || subcommand == "task" {
		t.mu.RLock()
		tasks := append([]*Task(nil), t.tasks[key]...)
		t.mu.RUnlock()
		for _, task := range tasks {
			if task != nil && task.TaskCancel != nil && task.TaskStatus == TaskStatusPending {
				task.TaskCancel()
				cancelledTasks++
			}
		}
	}

	if subcommand == "" || subcommand == "ai" {
		t.apiMu.Lock()
		cancels := t.apiCancels[key]
		for _, cancel := range cancels {
			cancel()
			cancelledAPI++
		}
		t.apiCancels[key] = nil
		t.apiMu.Unlock()
	}

	if cancelledTasks == 0 && cancelledAPI == 0 {
		return "该消息没有进行中的任务"
	}

	var parts []string
	if cancelledTasks > 0 {
		parts = append(parts, fmt.Sprintf("%d 个定时任务", cancelledTasks))
	}
	if cancelledAPI > 0 {
		parts = append(parts, fmt.Sprintf("%d 个 AI 请求", cancelledAPI))
	}
	return "已取消: " + strings.Join(parts, ", ")
}

func (t *TaskManager) executeTask(task *Task) {
	if task == nil {
		return
	}
	switch task.TaskType {
	case DelayDeleteTask:
		t.executeDelayDeleteTask(task)
	case DelayReplyTask:
		t.executeDelayReplyTask(task)
	default:
		t.updateTaskStatus(task, TaskStatusFailed)
		logger.Warn("unsupported task type: %s", task.TaskType)
	}
	t.removeTask(task)
}

func (t *TaskManager) executeDelayDeleteTask(task *Task) {
	defer task.TaskCancel()
	select {
	case <-time.After(task.Delay):
		delMsg := tgbotapi.NewDeleteMessage(task.Update.Message.Chat.ID, task.Update.Message.ReplyToMessage.MessageID)
		_, err := bot.Bot.Send(delMsg)
		if err != nil {
			t.updateTaskStatus(task, TaskStatusFailed)
			logger.Warn("Error in deleting msg: %s", err.Error())
			return
		}
		t.updateTaskStatus(task, TaskStatusDone)
	case <-task.TaskCtx.Done():
		t.updateTaskStatus(task, TaskStatusCancel)
		logger.Info("Del Task has been cancelled, task uuid: %s", task.TaskID)
	}
}

func (t *TaskManager) executeDelayReplyTask(task *Task) {
	defer task.TaskCancel()
	select {
	case <-time.After(task.Delay):
		replyMsg := tgbotapi.NewMessage(task.Update.Message.Chat.ID, task.ReplyText)
		replyMsg.ReplyToMessageID = task.Update.Message.ReplyToMessage.MessageID
		_, err := bot.Bot.Send(replyMsg)
		if err != nil {
			t.updateTaskStatus(task, TaskStatusFailed)
			logger.Warn("Error in sending delayed reply: %s", err.Error())
			return
		}
		t.updateTaskStatus(task, TaskStatusDone)
	case <-task.TaskCtx.Done():
		t.updateTaskStatus(task, TaskStatusCancel)
		logger.Info("Reply Task has been cancelled, task uuid: %s", task.TaskID)
	}
}

func (t *TaskManager) updateTaskStatus(task *Task, status TaskStatus) {
	t.mu.Lock()
	task.TaskStatus = status
	t.mu.Unlock()
}

func getTaskType(t string) TaskType {
	switch t {
	case "del":
		return DelayDeleteTask
	case "reply":
		return DelayReplyTask
	default:
		return ""
	}
}

func CheckPermission(taskType TaskType, u tgbotapi.Update) bool {
	ChatID := u.Message.Chat.ID
	UserID := u.Message.From.ID

	switch taskType {
	case DelayDeleteTask:
		return bot.Bot.IsAdminWithPermissions(ChatID, UserID, tgbotapi.AdminCanDeleteMessages) ||
			(u.Message.ReplyToMessage != nil && u.Message.From.ID == u.Message.ReplyToMessage.From.ID)
	case DelayReplyTask:
		return bot.Bot.IsAdmin(ChatID, UserID) ||
			(u.Message.ReplyToMessage != nil && u.Message.From.ID == u.Message.ReplyToMessage.From.ID)
	default:
		return bot.Bot.IsAdmin(ChatID, UserID)
	}
}

func CheckMemberStatus(u tgbotapi.Update) (string, error) {
	chatMember, err := bot.Bot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: u.Message.Chat.ID,
			UserID: u.Message.From.ID,
		},
	})
	if err != nil {
		logger.Error("Error in getting chatmember config")
		return "", err
	}
	return chatMember.Status, nil
}

func CheckBotOwner(u tgbotapi.Update) bool {
	if u.Message == nil || u.Message.From == nil {
		return false
	}
	return u.Message.From.ID == viper.GetInt64("BOT.owner_id")
}
