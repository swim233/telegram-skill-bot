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
	ReplyToUsername      string
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
	sb.WriteString("以下是群聊消息记录，请对这些消息进行总结：\n\n")
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
	sb.WriteString("\n请用中文总结以上群聊内容，包括：主要话题、活跃用户、重要讨论结论。")
	return sb.String(), nil
}
