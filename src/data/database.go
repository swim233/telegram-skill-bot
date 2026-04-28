package data

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/ijnkawakaze/telegram-bot-api"
	_ "github.com/mattn/go-sqlite3"
	"github.com/swim233/logger"
)

var db *sql.DB

type MessageRow struct {
	ChatID              int64
	MessageID           int
	UserID              int64
	Username            string
	UserFullName        string
	SenderTimeUTC       int64
	TextContent         string
	ImageBase64         string
	ImageMIME           string
	ReplyToMessageID    int
	ReplyToUsername     string
	ReplyToUserFullName string
	ReplyToTextContent  string
	ReplyToImageBase64  string
	ReplyToImageMIME    string
}

func InitDB() error {
	dbPath := "./data/chat_messages.db"
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	return createTable()
}

func createTable() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS group_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			user_full_name TEXT NOT NULL DEFAULT '',
			sender_time_utc INTEGER NOT NULL,
			text_content TEXT NOT NULL DEFAULT '',
			image_base64 TEXT NOT NULL DEFAULT '',
			image_mime TEXT NOT NULL DEFAULT '',
			reply_to_message_id INTEGER NOT NULL DEFAULT 0,
			reply_to_username TEXT NOT NULL DEFAULT '',
			reply_to_user_full_name TEXT NOT NULL DEFAULT '',
			reply_to_text_content TEXT NOT NULL DEFAULT '',
			reply_to_image_base64 TEXT NOT NULL DEFAULT '',
			reply_to_image_mime TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func SaveGroupMessage(update tgbotapi.Update) error {
	msg := update.Message
	if msg == nil {
		return nil
	}
	if msg.Chat == nil || msg.From == nil {
		return nil
	}

	row := MessageRow{
		ChatID:        msg.Chat.ID,
		MessageID:     msg.MessageID,
		UserID:        msg.From.ID,
		Username:      msg.From.UserName,
		UserFullName:  msg.From.FullName(),
		SenderTimeUTC: int64(msg.Date),
		TextContent:   strings.TrimSpace(msg.Text),
	}
	if row.TextContent == "" {
		row.TextContent = strings.TrimSpace(msg.Caption)
	}

	if msg.ReplyToMessage != nil {
		row.ReplyToMessageID = msg.ReplyToMessage.MessageID
		if msg.ReplyToMessage.From != nil {
			row.ReplyToUsername = msg.ReplyToMessage.From.UserName
			row.ReplyToUserFullName = msg.ReplyToMessage.From.FullName()
		}
		row.ReplyToTextContent = strings.TrimSpace(msg.ReplyToMessage.Text)
		if row.ReplyToTextContent == "" {
			row.ReplyToTextContent = strings.TrimSpace(msg.ReplyToMessage.Caption)
		}
	}

	_, err := db.Exec(`
		INSERT INTO group_messages
		(chat_id, message_id, user_id, username, user_full_name, sender_time_utc, text_content, image_base64, image_mime,
		reply_to_message_id, reply_to_username, reply_to_user_full_name, reply_to_text_content, reply_to_image_base64, reply_to_image_mime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ChatID,
		row.MessageID,
		row.UserID,
		row.Username,
		row.UserFullName,
		row.SenderTimeUTC,
		row.TextContent,
		row.ImageBase64,
		row.ImageMIME,
		row.ReplyToMessageID,
		row.ReplyToUsername,
		row.ReplyToUserFullName,
		row.ReplyToTextContent,
		row.ReplyToImageBase64,
		row.ReplyToImageMIME,
	)
	if err != nil {
		logger.Error("Error saving message: %s", err.Error())
	}
	return err
}

func BuildSummaryPrompt(chatID int64, duration time.Duration) (string, error) {
	since := time.Now().UTC().Add(-duration).Unix()
	rows, err := db.Query(`
		SELECT user_full_name, text_content, sender_time_utc, reply_to_user_full_name, reply_to_text_content
		FROM group_messages
		WHERE chat_id = ? AND sender_time_utc >= ? AND text_content != ''
		ORDER BY sender_time_utc ASC`,
		chatID, since)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(`你现在是一个专业且有趣的群聊内容总结助手，专门负责处理各种微信/QQ/Telegram群聊记录。

你的任务是：接收一段包含以下格式的群聊消息：
[时间] 发言人 : 发言内容
或包含回复格式：
↳ 回复 @某人: 回复内容

请你用**中文**对整个群聊内容进行总结，语言风格要**略带挑逗、暧昧、坏坏的语气**，既要总结清楚，又要读起来带点刺激感和趣味性。

### 总结必须包含以下核心部分（按此顺序组织）：

1. **主要话题**  
   用稍微撩人的语气点出本次群聊最主要的1-3个话题，顺便带点调侃或暗示。

2. **活跃用户**  
   列出本次聊天中最活跃的3-6位用户（按活跃度排序），并用带点挑逗的笔触描述他们的表现（例如：谁最会撩、谁最敢开车、谁一直在拱火、谁突然变得很主动等）。

3. **重要讨论内容与结论**  
   梳理关键讨论过程，重点提取：
   - 各方主要观点和有趣的分歧
   - 最终达成的共识或重要结论
   - 尚未解决但让人浮想联翩的内容
   - 任何具体决定、行动点或结果

4. **色情/开车/刺激内容**  
   对于群内出现的色情、黄腔、开车、暧昧、R18相关内容，允许且需要**大胆、直接、带点挑逗地描述**，不要过度净化，可以适度保留原有的刺激感和画面感，但要控制在总结的合理尺度内。

5. **群内氛围**  
   描述本次群聊的整体情绪氛围（是欢乐开车、集体发情、暗戳戳互撩、还是表面正经其实很色等），语气要带点坏笑的感觉。

### 总结要求：
- 整体语言风格要**略带挑逗、轻微暧昧、坏坏的**，可以适当使用一些带感、撩人的表达，但不要过于低俗或下流。
- 使用 bullet points 或编号列表，让结构清晰又好看。
- 如果内容较多，可按话题分段总结，并标注大致时间段。
- 总结长度要适中，既完整又不冗长。
- 最后用一句**带点挑逗意味的总结金句**收尾，概括这次群聊的精髓。

现在，请开始总结我发给你的群聊记录吧～

`)
	count := 0
	for rows.Next() {
		var fullName, text, replyFullName, replyText string
		var ts int64
		if err := rows.Scan(&fullName, &text, &ts, &replyFullName, &replyText); err != nil {
			logger.Error("Error scanning row: %s", err.Error())
			continue
		}
		timeStr := time.Unix(ts, 0).UTC().Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", timeStr, fullName, text))
		if replyFullName != "" && replyText != "" {
			sb.WriteString(fmt.Sprintf("  ↳ 回复 %s: %s\n", replyFullName, replyText))
		}
		count++
	}
	if count == 0 {
		return "", nil
	}
	return sb.String(), nil
}
