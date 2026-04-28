package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/swim233/chat_bot/bot"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	tgbotapi "github.com/ijnkawakaze/telegram-bot-api"
	"github.com/spf13/viper"
	"github.com/swim233/chat_bot/lib"
	"github.com/swim233/logger"
	"resty.dev/v3"
)

type RequestRoute struct {
	Endpoint    string
	Token       string
	RequestType string
	Model       string
}

type SkillRequestInput struct {
	Prompt       string
	ReplyContent string
	FileName     string
	FileData     []byte
	MimeType     string
}

func SendRequest(ctx context.Context, input string) (output string, err error) {
	route, err := resolveRouteByScene("summary")
	if err != nil {
		return "", err
	}
	return sendRequestWithRoute(ctx, input, route)
}

func SendRequestByScene(ctx context.Context, input, scene string) (string, error) {
	route, err := resolveRouteByScene(scene)
	if err != nil {
		return "", err
	}
	return sendRequestWithRoute(ctx, input, route)
}

func SendSkillRequest(ctx context.Context, update tgbotapi.Update) (string, error) {
	if update.Message == nil {
		return "", fmt.Errorf("invalid message")
	}
	input, err := buildSkillRequestInput(update)
	if err != nil {
		return "", err
	}
	route, err := resolveRouteByScene("skill")
	if err != nil {
		return "", err
	}
	return sendSkillRequestWithRoute(ctx, input, route)
}

func sendRequestWithRoute(ctx context.Context, input string, route RequestRoute) (output string, err error) {
	if route.Endpoint == "" || route.Token == "" {
		return "", fmt.Errorf("invalid route config: endpoint/token is empty")
	}
	requestType := strings.ToLower(strings.TrimSpace(route.RequestType))
	if requestType == "" {
		requestType = "responses"
	}
	client := openai.NewClient(
		option.WithBaseURL(route.Endpoint),
		option.WithAPIKey(route.Token),
		option.WithHeader("User-Agent", "claude-code/2.1.101"),
	)
	switch requestType {
	case "responses":
		start := time.Now()
		logger.Info("HTTP request start: api_type=responses endpoint=%s model=%s token=%s input_chars=%d", route.Endpoint, route.Model, maskToken(route.Token), len(input))
		rsp, err := client.Responses.New(ctx, responses.ResponseNewParams{
			Input: responses.ResponseNewParamsInputUnion{
				OfString: openai.String(input),
			},
			Model: route.Model,
		})
		if err != nil {
			logger.Error("HTTP request failed: api_type=responses endpoint=%s elapsed=%s err=%s", route.Endpoint, time.Since(start), err.Error())
			return "", err
		}
		logger.Info("HTTP request done: api_type=responses endpoint=%s elapsed=%s output_chars=%d", route.Endpoint, time.Since(start), len(rsp.OutputText()))
		logger.Info("token usage: api_type=responses input_tokens=%d output_tokens=%d total_tokens=%d", rsp.Usage.InputTokens, rsp.Usage.OutputTokens, rsp.Usage.TotalTokens)
		logger.Info("Get response: %s", rsp.OutputText())
		return rsp.OutputText(), nil
	case "completions":
		start := time.Now()
		logger.Info("HTTP request start: api_type=completions endpoint=%s model=%s token=%s input_chars=%d", route.Endpoint, route.Model, maskToken(route.Token), len(input))
		rsp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model: route.Model,
			Messages: []openai.ChatCompletionMessageParamUnion{
				{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfString: openai.String(input),
						},
					},
				},
			},
		})
		if err != nil {
			logger.Error("HTTP request failed: api_type=completions endpoint=%s elapsed=%s err=%s", route.Endpoint, time.Since(start), err.Error())
			return "", err
		}
		logger.Info("HTTP request done: api_type=completions endpoint=%s elapsed=%s choices=%d", route.Endpoint, time.Since(start), len(rsp.Choices))
		if len(rsp.Choices) == 0 {
			return "", nil
		}
		logger.Info("token usage: api_type=completions prompt_tokens=%d completion_tokens=%d total_tokens=%d", rsp.Usage.PromptTokens, rsp.Usage.CompletionTokens, rsp.Usage.TotalTokens)
		return rsp.Choices[0].Message.Content, nil
	default:
		return "", fmt.Errorf("unsupported API.request_type: %s", requestType)
	}
}

func sendSkillRequestWithRoute(ctx context.Context, input SkillRequestInput, route RequestRoute) (output string, err error) {
	if route.Endpoint == "" || route.Token == "" {
		return "", fmt.Errorf("invalid route config: endpoint/token is empty")
	}
	requestType := strings.ToLower(strings.TrimSpace(route.RequestType))
	if requestType == "" {
		requestType = "responses"
	}
	if len(input.FileData) > 0 && requestType != "responses" && detectSkillFileKind(input.MimeType) != skillFileKindImage {
		return "", fmt.Errorf("skill 命令上传非图片文件时仅支持 responses 请求类型")
	}
	// 图片附件强制走 completions（部分转发 API 的 responses 端点不支持图片）
	if len(input.FileData) > 0 && detectSkillFileKind(input.MimeType) == skillFileKindImage && requestType == "responses" {
		requestType = "completions"
	}

	client := openai.NewClient(
		option.WithBaseURL(route.Endpoint),
		option.WithAPIKey(route.Token),
		option.WithHeader("User-Agent", "claude-code/2.1.101"),
	)

	promptText := buildSkillPrompt(input)
	fileKind := detectSkillFileKind(input.MimeType)
	if len(input.FileData) > 0 && fileKind == skillFileKindOther {
		start := time.Now()
		logger.Info("HTTP request start: api_type=files endpoint=%s token=%s filename=%s file_bytes=%d", route.Endpoint, maskToken(route.Token), input.FileName, len(input.FileData))
		fileObj, err := client.Files.New(ctx, openai.FileNewParams{
			File:    bytes.NewReader(input.FileData),
			Purpose: openai.FilePurposeUserData,
		})
		if err != nil {
			logger.Error("HTTP request failed: api_type=files endpoint=%s elapsed=%s err=%s", route.Endpoint, time.Since(start), err.Error())
			return "", err
		}
		logger.Info("HTTP request done: api_type=files endpoint=%s elapsed=%s file_id=%s", route.Endpoint, time.Since(start), fileObj.ID)
	}

	switch requestType {
	case "responses":
		var content responses.ResponseInputMessageContentListParam
		content = append(content, responses.ResponseInputContentParamOfInputText(promptText))
		if len(input.FileData) > 0 {
			if fileKind == skillFileKindImage {
				content = append(content, responses.ResponseInputContentUnionParam{
					OfInputImage: &responses.ResponseInputImageParam{
						ImageURL: openai.String("data:" + input.MimeType + ";base64," + base64.StdEncoding.EncodeToString(input.FileData)),
						Detail:   responses.ResponseInputImageDetailAuto,
					},
				})
			} else {
				content = append(content, responses.ResponseInputContentUnionParam{
					OfInputFile: &responses.ResponseInputFileParam{
						FileData: openai.String(base64.StdEncoding.EncodeToString(input.FileData)),
						Filename: openai.String(input.FileName),
						Detail:   responses.ResponseInputFileDetailLow,
					},
				})
			}
		}
		start := time.Now()
		logger.Info("HTTP request start: api_type=responses endpoint=%s model=%s token=%s input_chars=%d file_bytes=%d", route.Endpoint, route.Model, maskToken(route.Token), len(promptText), len(input.FileData))
		rsp, err := client.Responses.New(ctx, responses.ResponseNewParams{
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: []responses.ResponseInputItemUnionParam{
					responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser),
				},
			},
			Model: route.Model,
		})
		if err != nil {
			logger.Error("HTTP request failed: api_type=responses endpoint=%s elapsed=%s err=%s", route.Endpoint, time.Since(start), err.Error())
			if len(input.FileData) == 0 {
				logger.Warn("skill responses failed, fallback to completions: endpoint=%s err=%s", route.Endpoint, err.Error())
				return sendSkillRequestWithRouteAsCompletions(ctx, promptText, input, route, fileKind)
			}
			return "", err
		}
		logger.Info("HTTP request done: api_type=responses endpoint=%s elapsed=%s output_chars=%d", route.Endpoint, time.Since(start), len(rsp.OutputText()))
		logger.Info("token usage: api_type=responses input_tokens=%d output_tokens=%d total_tokens=%d", rsp.Usage.InputTokens, rsp.Usage.OutputTokens, rsp.Usage.TotalTokens)
		logger.Info("Get response: %s", rsp.OutputText())
		return rsp.OutputText(), nil
	case "completions":
		msgContent := []openai.ChatCompletionContentPartUnionParam{
			openai.TextContentPart(promptText),
		}
		if len(input.FileData) > 0 {
			switch fileKind {
			case skillFileKindImage:
				msgContent = append(msgContent, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL:    "data:" + input.MimeType + ";base64," + base64.StdEncoding.EncodeToString(input.FileData),
					Detail: "auto",
				}))
			case skillFileKindOther:
				msgContent = append(msgContent, openai.FileContentPart(openai.ChatCompletionContentPartFileFileParam{
					FileData: openai.String(base64.StdEncoding.EncodeToString(input.FileData)),
					Filename: openai.String(input.FileName),
				}))
			default:
				return "", fmt.Errorf("无法识别附件类型，无法构造 skill 请求")
			}
		}
		start := time.Now()
		logger.Info("HTTP request start: api_type=completions endpoint=%s model=%s token=%s input_chars=%d file_bytes=%d", route.Endpoint, route.Model, maskToken(route.Token), len(promptText), len(input.FileData))
		rsp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model: route.Model,
			Messages: []openai.ChatCompletionMessageParamUnion{
				{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfArrayOfContentParts: msgContent,
						},
					},
				},
			},
		})
		if err != nil {
			logger.Error("HTTP request failed: api_type=completions endpoint=%s elapsed=%s err=%s", route.Endpoint, time.Since(start), err.Error())
			return "", err
		}
		logger.Info("HTTP request done: api_type=completions endpoint=%s elapsed=%s choices=%d", route.Endpoint, time.Since(start), len(rsp.Choices))
		if len(rsp.Choices) == 0 {
			return "", nil
		}
		logger.Info("token usage: api_type=completions prompt_tokens=%d completion_tokens=%d total_tokens=%d", rsp.Usage.PromptTokens, rsp.Usage.CompletionTokens, rsp.Usage.TotalTokens)
		logger.Info("Get response: %s", rsp.Choices[0].Message.Content)
		return rsp.Choices[0].Message.Content, nil
	default:
		return "", fmt.Errorf("unsupported API.request_type: %s", requestType)
	}
}

func sendSkillRequestWithRouteAsCompletions(ctx context.Context, promptText string, input SkillRequestInput, route RequestRoute, fileKind skillFileKind) (string, error) {
	client := openai.NewClient(
		option.WithBaseURL(route.Endpoint),
		option.WithAPIKey(route.Token),
		option.WithHeader("User-Agent", "claude-code/2.1.101"),
	)
	msgContent := []openai.ChatCompletionContentPartUnionParam{
		openai.TextContentPart(promptText),
	}
	if len(input.FileData) > 0 {
		switch fileKind {
		case skillFileKindImage:
			msgContent = append(msgContent, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL:    "data:" + input.MimeType + ";base64," + base64.StdEncoding.EncodeToString(input.FileData),
				Detail: "auto",
			}))
		case skillFileKindOther:
			msgContent = append(msgContent, openai.FileContentPart(openai.ChatCompletionContentPartFileFileParam{
				FileData: openai.String(base64.StdEncoding.EncodeToString(input.FileData)),
				Filename: openai.String(input.FileName),
			}))
		}
	}
	start := time.Now()
	logger.Info("HTTP request start: api_type=completions endpoint=%s model=%s token=%s input_chars=%d file_bytes=%d", route.Endpoint, route.Model, maskToken(route.Token), len(promptText), len(input.FileData))
	rsp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: route.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfArrayOfContentParts: msgContent,
					},
				},
			},
		},
	})
	if err != nil {
		logger.Error("HTTP request failed: api_type=completions endpoint=%s elapsed=%s err=%s", route.Endpoint, time.Since(start), err.Error())
		return "", err
	}
	logger.Info("HTTP request done: api_type=completions endpoint=%s elapsed=%s choices=%d", route.Endpoint, time.Since(start), len(rsp.Choices))
	if len(rsp.Choices) == 0 {
		return "", nil
	}
	logger.Info("token usage: api_type=completions prompt_tokens=%d completion_tokens=%d total_tokens=%d", rsp.Usage.PromptTokens, rsp.Usage.CompletionTokens, rsp.Usage.TotalTokens)
	return rsp.Choices[0].Message.Content, nil
}

type skillFileKind string

const (
	skillFileKindNone   skillFileKind = "none"
	skillFileKindImage  skillFileKind = "image"
	skillFileKindOther  skillFileKind = "other"
)

func detectSkillFileKind(mimeType string) skillFileKind {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "" {
		return skillFileKindNone
	}
	if strings.HasPrefix(mimeType, "image/") {
		return skillFileKindImage
	}
	return skillFileKindOther
}

func buildSkillRequestInput(update tgbotapi.Update) (SkillRequestInput, error) {
	msg := update.Message
	if msg == nil {
		return SkillRequestInput{}, fmt.Errorf("invalid message")
	}
	input := SkillRequestInput{
		Prompt: strings.TrimSpace(msg.CommandArguments()),
	}
	if input.Prompt == "" {
		input.Prompt = strings.TrimSpace(stripCommandPrefix(msg.Text, "skill"))
	}
	if input.Prompt == "" {
		input.Prompt = strings.TrimSpace(stripCommandPrefix(msg.Caption, "skill"))
	}
	if input.Prompt == "" && (msg.ReplyToMessage != nil || len(msg.Photo) > 0 || msg.Document != nil) {
		input.Prompt = "请分析这个内容"
	}
	if msg.ReplyToMessage != nil {
		input.ReplyContent = formatTelegramMessage(msg.ReplyToMessage)
	}
	fileName, fileData, mimeType, err := extractSkillAttachment(msg)
	if err != nil {
		return SkillRequestInput{}, err
	}
	if len(fileData) == 0 && msg.ReplyToMessage != nil {
		fileName, fileData, mimeType, err = extractSkillAttachment(msg.ReplyToMessage)
		if err != nil {
			return SkillRequestInput{}, err
		}
	}
	input.FileName = fileName
	input.FileData = fileData
	input.MimeType = mimeType
	return input, nil
}

func stripCommandPrefix(text, command string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return text
	}
	first := strings.TrimPrefix(fields[0], "/")
	first = strings.TrimPrefix(first, "@")
	if idx := strings.Index(first, "@"); idx >= 0 {
		first = first[:idx]
	}
	if strings.EqualFold(first, command) {
		return strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	}
	return text
}

func buildSkillPrompt(input SkillRequestInput) string {
	var sb strings.Builder
	sb.WriteString("# 角色设定\n")
	sb.WriteString("你是一个专业的群聊技能助手。你需要根据用户的提问、回复上下文和附件内容，直接给出可执行、可验证的答案。\n\n")
	sb.WriteString("# 任务要求\n")
	sb.WriteString("1. 优先回答用户当前问题。\n")
	sb.WriteString("2. 如果存在回复消息，把回复内容视为上下文，不要重复赘述。\n")
	sb.WriteString("3. 如果存在附件，请结合附件内容分析。\n")
	sb.WriteString("4. 输出要简洁，优先给结论和步骤，不要长篇解释。\n")
	sb.WriteString("5. 如果附件内容无法识别，直接说明原因。\n\n")
	if input.ReplyContent != "" {
		sb.WriteString("# 回复上下文\n")
		sb.WriteString(input.ReplyContent)
		sb.WriteString("\n\n")
	}
	if input.FileName != "" {
		sb.WriteString("# 附件信息\n")
		sb.WriteString("文件名: ")
		sb.WriteString(input.FileName)
		if input.MimeType != "" {
			sb.WriteString("\n类型: ")
			sb.WriteString(input.MimeType)
		}
		sb.WriteString("\n\n")
	}
	sb.WriteString("# 用户问题\n")
	sb.WriteString(input.Prompt)
	return sb.String()
}

func extractSkillAttachment(msg *tgbotapi.Message) (fileName string, fileData []byte, mimeType string, err error) {
	const maxAttachmentSize = 5 * 1024 * 1024
	switch {
	case msg.Document != nil:
		if msg.Document.FileSize > maxAttachmentSize {
			return "", nil, "", fmt.Errorf("附件超过 5MB，当前大小: %d bytes", msg.Document.FileSize)
		}
		fileName = msg.Document.FileName
		if fileName == "" {
			fileName = msg.Document.FileID
		}
		return downloadTelegramFile(msg.Document.FileID, fileName, msg.Document.MimeType)
	case len(msg.Photo) > 0:
		photo := msg.Photo[len(msg.Photo)-1]
		if photo.FileSize > maxAttachmentSize {
			return "", nil, "", fmt.Errorf("图片超过 5MB，当前大小: %d bytes", photo.FileSize)
		}
		fileName = photo.FileID + ".jpg"
		return downloadTelegramFile(photo.FileID, fileName, "image/jpeg")
	case msg.Video != nil:
		if msg.Video.FileSize > maxAttachmentSize {
			return "", nil, "", fmt.Errorf("视频超过 5MB，当前大小: %d bytes", msg.Video.FileSize)
		}
		fileName = msg.Video.FileName
		if fileName == "" {
			fileName = msg.Video.FileID
		}
		return downloadTelegramFile(msg.Video.FileID, fileName, msg.Video.MimeType)
	case msg.Audio != nil:
		if msg.Audio.FileSize > maxAttachmentSize {
			return "", nil, "", fmt.Errorf("音频超过 5MB，当前大小: %d bytes", msg.Audio.FileSize)
		}
		fileName = msg.Audio.FileName
		if fileName == "" {
			fileName = msg.Audio.FileID
		}
		return downloadTelegramFile(msg.Audio.FileID, fileName, msg.Audio.MimeType)
	case msg.Voice != nil:
		if msg.Voice.FileSize > maxAttachmentSize {
			return "", nil, "", fmt.Errorf("语音超过 5MB，当前大小: %d bytes", msg.Voice.FileSize)
		}
		fileName = msg.Voice.FileID + ".ogg"
		return downloadTelegramFile(msg.Voice.FileID, fileName, msg.Voice.MimeType)
	default:
		return "", nil, "", nil
	}
}

func downloadTelegramFile(fileID, fileName, mimeType string) (string, []byte, string, error) {
	if fileID == "" {
		return "", nil, "", nil
	}
	fileURL, err := bot.Bot.GetFileDirectURL(fileID)
	if err != nil {
		return "", nil, "", err
	}
	start := time.Now()
	logger.Info("HTTP request start: method=GET url=%s", fileURL)
	resp, err := http.Get(fileURL)
	if err != nil {
		logger.Error("HTTP request failed: method=GET url=%s elapsed=%s err=%s", fileURL, time.Since(start), err.Error())
		return "", nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, "", err
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	logger.Info("HTTP request done: method=GET url=%s elapsed=%s file_bytes=%d", fileURL, time.Since(start), len(data))
	return fileName, data, mimeType, nil
}

func formatTelegramMessage(msg *tgbotapi.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	if from := msg.From; from != nil {
		sb.WriteString("发言人: ")
		sb.WriteString(from.FullName())
		if from.UserName != "" {
			sb.WriteString(" (@")
			sb.WriteString(from.UserName)
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}
	if text := strings.TrimSpace(msg.Text); text != "" {
		sb.WriteString("内容: ")
		sb.WriteString(text)
		sb.WriteString("\n")
	} else if caption := strings.TrimSpace(msg.Caption); caption != "" {
		sb.WriteString("内容: ")
		sb.WriteString(caption)
		sb.WriteString("\n")
	}
	switch {
	case msg.Document != nil:
		sb.WriteString("附件: document ")
		sb.WriteString(msg.Document.FileName)
		sb.WriteString("\n")
	case len(msg.Photo) > 0:
		sb.WriteString("附件: photo\n")
	case msg.Video != nil:
		sb.WriteString("附件: video ")
		sb.WriteString(msg.Video.FileName)
		sb.WriteString("\n")
	case msg.Audio != nil:
		sb.WriteString("附件: audio ")
		sb.WriteString(msg.Audio.FileName)
		sb.WriteString("\n")
	case msg.Voice != nil:
		sb.WriteString("附件: voice\n")
	}
	return strings.TrimSpace(sb.String())
}

func resolveRouteByScene(scene string) (RequestRoute, error) {
	cfg := lib.Config{}
	if err := viper.Unmarshal(&cfg); err != nil {
		return RequestRoute{}, err
	}
	if len(cfg.API.APIs) == 0 {
		return RequestRoute{}, fmt.Errorf("API.apis is not configured")
	}
	scene = strings.ToLower(strings.TrimSpace(scene))
	var targetAPI string
	var targetToken int
	var targetModel string
	switch scene {
	case "summary":
		targetAPI = strings.TrimSpace(cfg.API.SummaryAPI)
		targetToken = cfg.API.SummaryToken
		targetModel = strings.TrimSpace(cfg.API.SummaryModule)
	case "skill":
		targetAPI = strings.TrimSpace(cfg.API.SkillAPI)
		targetToken = cfg.API.SkillToken
		targetModel = strings.TrimSpace(cfg.API.SkillModule)
	case "focus":
		targetAPI = strings.TrimSpace(cfg.API.FocusAPI)
		targetToken = cfg.API.FocusToken
		targetModel = strings.TrimSpace(cfg.API.FocusModule)
	default:
		return RequestRoute{}, fmt.Errorf("unsupported scene: %s", scene)
	}
	if targetModel == "" {
		return RequestRoute{}, fmt.Errorf("scene %s model is not configured (set %s_module in config)", scene, scene)
	}

	selected := cfg.API.APIs[0]
	if targetAPI != "" {
		found := false
		for _, p := range cfg.API.APIs {
			if strings.EqualFold(strings.TrimSpace(p.Name), targetAPI) {
				selected = p
				found = true
				break
			}
		}
		if !found {
			return RequestRoute{}, fmt.Errorf("scene %s api not found: %s", scene, targetAPI)
		}
	}
	if len(selected.Tokens) == 0 {
		return RequestRoute{}, fmt.Errorf("api %s has no tokens", selected.Name)
	}
	if targetToken < 0 || targetToken >= len(selected.Tokens) {
		targetToken = 0
	}
	secret := strings.TrimSpace(selected.Tokens[targetToken].Secret())
	if secret == "" {
		return RequestRoute{}, fmt.Errorf("api %s token[%d] is empty", selected.Name, targetToken)
	}
	reqType := strings.ToLower(strings.TrimSpace(selected.RequestType))
	if reqType == "" {
		reqType = "responses"
	}
	return RequestRoute{
		Endpoint:    strings.TrimSpace(selected.Endpoint),
		Token:       secret,
		RequestType: reqType,
		Model:       targetModel,
	}, nil
}

func GetModelsByScene(scene string) ([]string, error) {
	route, err := resolveRouteByScene(scene)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	url := strings.TrimRight(route.Endpoint, "/") + "/models"
	logger.Info("HTTP request start: method=GET url=%s scene=%s", url, scene)
	client := resty.New().SetAuthToken(route.Token)
	var models lib.Models
	rsp, err := client.R().
		SetHeader("Accept", "application/json").
		SetResult(&models).
		Get(url)
	if err != nil {
		logger.Error("HTTP request failed: method=GET url=%s scene=%s elapsed=%s err=%s", url, scene, time.Since(start), err.Error())
		return nil, err
	}
	if rsp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get models failed, scene=%s status=%d body=%s", scene, rsp.StatusCode(), rsp.String())
	}
	logger.Info("HTTP request done: method=GET url=%s scene=%s elapsed=%s models=%d", url, scene, time.Since(start), len(models.Data))
	out := make([]string, 0, len(models.Data))
	for _, m := range models.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out, nil
}

func maskToken(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
