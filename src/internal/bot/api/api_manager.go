package apiConfig

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/ijnkawakaze/telegram-bot-api"
	"github.com/spf13/viper"
	"github.com/swim233/chat_bot/api"
	"github.com/swim233/chat_bot/config"
	"github.com/swim233/chat_bot/internal/bot/task"
	"github.com/swim233/chat_bot/lib"
)

func SwitchAction(u tgbotapi.Update) (string, error) {
	if !task.CheckBotOwner(u) && (u.Message == nil || !config.HasPermission(u.Message.Chat.ID, u.Message.From.ID, "switch")) {
		return "", errors.New("无权使用 /switch，请联系 owner 授权")
	}
	if u.Message == nil {
		return "", errors.New("invalid message")
	}
	args := strings.Fields(u.Message.CommandArguments())
	if len(args) < 2 {
		return "", errors.New("用法: /switch models <skill|summary> <model_id> | /switch api <skill|summary> <api_name> [token_index]")
	}
	switch strings.ToLower(args[0]) {
	case "models", "model":
		if len(args) < 3 {
			return "", errors.New("用法: /switch models <skill|summary> <model_id>")
		}
		return switchModelForScene(args[1], args[2])
	case "api":
		if len(args) < 3 {
			return "", errors.New("用法: /switch api <skill|summary> <api_name> [token_index]")
		}
		tokenIndex, err := parseTokenIndex(args, 3)
		if err != nil {
			return "", err
		}
		return switchSceneAPI(args[1], args[2], tokenIndex)
	case "summary_api", "summary-api", "summaryapi":
		if len(args) < 2 {
			return "", errors.New("用法: /switch api summary <api_name> [token_index]")
		}
		tokenIndex, err := parseTokenIndex(args, 2)
		if err != nil {
			return "", err
		}
		return switchSceneAPI("summary", args[1], tokenIndex)
	case "skill_api", "skill-api", "skillapi":
		if len(args) < 2 {
			return "", errors.New("用法: /switch api skill <api_name> [token_index]")
		}
		tokenIndex, err := parseTokenIndex(args, 2)
		if err != nil {
			return "", err
		}
		return switchSceneAPI("skill", args[1], tokenIndex)
	default:
		return "", errors.New("未知子命令，仅支持 models/api")
	}
}

func parseTokenIndex(args []string, tokenArgIndex int) (int, error) {
	tokenIndex := 0
	if len(args) > tokenArgIndex {
		idx, err := strconv.Atoi(args[tokenArgIndex])
		if err != nil || idx < 0 {
			return 0, errors.New("token_index 必须是 >=0 的整数")
		}
		tokenIndex = idx
	}
	return tokenIndex, nil
}

func switchModelForScene(scene, modelID string) (string, error) {
	scene = strings.ToLower(strings.TrimSpace(scene))
	if scene != "skill" && scene != "summary" {
		return "", errors.New("models 场景仅支持 skill 或 summary")
	}
	models, err := api.GetModelsByScene(scene)
	if err != nil {
		return "", err
	}
	exists := false
	for _, item := range models {
		if strings.TrimSpace(item) == strings.TrimSpace(modelID) {
			exists = true
			break
		}
	}
	if !exists {
		return "", fmt.Errorf("model not found: %s", modelID)
	}

	cfg := lib.Config{}
	if err := viper.Unmarshal(&cfg); err != nil {
		return "", err
	}
	switch scene {
	case "skill":
		cfg.API.SkillModule = modelID
	case "summary":
		cfg.API.SummaryModule = modelID
	}
	if err := config.ChangeConfig(&cfg); err != nil {
		return "", err
	}
	if err := viper.ReadInConfig(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s 场景模型已切换至: %s", scene, modelID), nil
}

func switchSceneAPI(scene, apiName string, tokenIndex int) (string, error) {
	cfg := lib.Config{}
	if err := viper.Unmarshal(&cfg); err != nil {
		return "", err
	}
	if len(cfg.API.APIs) == 0 {
		return "", errors.New("未配置 API.apis")
	}

	foundIdx := -1
	for i, item := range cfg.API.APIs {
		if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(apiName)) {
			foundIdx = i
			break
		}
	}
	if foundIdx < 0 {
		return "", fmt.Errorf("api not found: %s", apiName)
	}
	selected := cfg.API.APIs[foundIdx]
	if len(selected.Tokens) == 0 {
		return "", fmt.Errorf("api %s 没有可用 token", selected.Name)
	}
	if tokenIndex < 0 || tokenIndex >= len(selected.Tokens) {
		return "", fmt.Errorf("token_index 越界，可用范围: 0-%d", len(selected.Tokens)-1)
	}
	if strings.TrimSpace(selected.Endpoint) == "" {
		return "", fmt.Errorf("api %s endpoint 为空", selected.Name)
	}

	switch scene {
	case "summary":
		cfg.API.SummaryAPI = selected.Name
		cfg.API.SummaryToken = tokenIndex
	case "skill":
		cfg.API.SkillAPI = selected.Name
		cfg.API.SkillToken = tokenIndex
	default:
		return "", fmt.Errorf("unsupported scene: %s", scene)
	}

	if err := config.ChangeConfig(&cfg); err != nil {
		return "", err
	}
	if err := viper.ReadInConfig(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s 场景 API 已切换至: %s (token[%d])", scene, selected.Name, tokenIndex), nil
}

func ListAPIWithMask() (string, error) {
	cfg := lib.Config{}
	if err := viper.Unmarshal(&cfg); err != nil {
		return "", err
	}
	if len(cfg.API.APIs) == 0 {
		return "未配置 API.apis", nil
	}
	var sb strings.Builder
	sb.WriteString("可用 API 列表\n")
	sb.WriteString("========\n")
	for _, item := range cfg.API.APIs {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "(unnamed)"
		}
		sb.WriteString("\n")
		sb.WriteString("API: " + name)
		var scenes []string
		if strings.EqualFold(name, strings.TrimSpace(cfg.API.SummaryAPI)) {
			scenes = append(scenes, "summary")
		}
		if strings.EqualFold(name, strings.TrimSpace(cfg.API.SkillAPI)) {
			scenes = append(scenes, "skill")
		}
		if len(scenes) > 0 {
			sb.WriteString("  [" + strings.Join(scenes, ", ") + "]")
		}
		sb.WriteString("\n")
		sb.WriteString("endpoint: " + item.Endpoint + "\n")
		reqType := strings.ToLower(strings.TrimSpace(item.RequestType))
		if reqType == "" {
			reqType = "responses"
		}
		sb.WriteString("request_type: " + reqType + "\n")
		if len(item.Tokens) == 0 {
			sb.WriteString("tokens: (none)\n")
			continue
		}
		sb.WriteString("tokens:\n")
		for i, token := range item.Tokens {
			masked := maskSecret(token.Secret())
			tag := strings.TrimSpace(token.Tag)
			if tag == "" {
				tag = "untagged"
			}
			var active []string
			if strings.EqualFold(name, strings.TrimSpace(cfg.API.SummaryAPI)) && i == cfg.API.SummaryToken {
				active = append(active, "summary")
			}
			if strings.EqualFold(name, strings.TrimSpace(cfg.API.SkillAPI)) && i == cfg.API.SkillToken {
				active = append(active, "skill")
			}
			suffix := ""
			if len(active) > 0 {
				suffix = " [" + strings.Join(active, ", ") + "]"
			}
			fmt.Fprintf(&sb, "  - [%d] tag=%s, key=%s%s\n", i, tag, masked, suffix)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	r := []rune(s)
	if len(r) <= 8 {
		return "****"
	}
	return string(r[:4]) + "****" + string(r[len(r)-4:])
}
