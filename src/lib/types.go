package lib

// Config 顶层配置
type Config struct {
	API  APIConfig  `yaml:"API"`
	BOT  BOTConfig  `yaml:"BOT"`
	DATA DataConfig `yaml:"DATA"`
}

// APIConfig API 相关配置
type APIConfig struct {
	SummaryModule string        `yaml:"summary_module" mapstructure:"summary_module"`
	SkillModule   string        `yaml:"skill_module" mapstructure:"skill_module"`
	SummaryAPI    string        `yaml:"summary_api" mapstructure:"summary_api"`
	SummaryToken  int           `yaml:"summary_token" mapstructure:"summary_token"`
	SkillAPI      string        `yaml:"skill_api" mapstructure:"skill_api"`
	SkillToken    int           `yaml:"skill_token" mapstructure:"skill_token"`
	APIs          []APIProvider `yaml:"apis" mapstructure:"apis"`
}

type APIProvider struct {
	Name        string     `yaml:"name" mapstructure:"name"`
	Endpoint    string     `yaml:"endpoint" mapstructure:"endpoint"`
	RequestType string     `yaml:"request_type" mapstructure:"request_type"`
	Tokens      []APIToken `yaml:"tokens" mapstructure:"tokens"`
}

type APIToken struct {
	Tag   string `yaml:"tag" mapstructure:"tag"`
	Token string `yaml:"token" mapstructure:"token"`
}

func (t APIToken) Secret() string {
	return t.Token
}

type BOTConfig struct {
	BotToken      string   `yaml:"bot_token" mapstructure:"bot_token"`
	OwnerID       int64    `yaml:"owner_id" mapstructure:"owner_id"`
	AllowList     []int64  `yaml:"allow_list" mapstructure:"allow_list"`
	AllowCommands []string `yaml:"allow_commands" mapstructure:"allow_commands"`
}

type DataConfig struct {
	SQLitePath string `yaml:"sqlite_path" mapstructure:"sqlite_path"`
}

type Models struct {
	Data   []Datum `json:"data"`
	Object string  `json:"object"`
}

type Datum struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
