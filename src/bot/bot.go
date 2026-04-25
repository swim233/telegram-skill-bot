package bot

import (
	tgbotapi "github.com/ijnkawakaze/telegram-bot-api"
	viper "github.com/spf13/viper"
	"github.com/swim233/logger"
)

type botConfig struct {
	Token string
}

var Bot *tgbotapi.BotAPI
var BotConfig botConfig

func InitBot() {

	BotConfig.Token = viper.GetString("BOT.bot_token")
	if BotConfig.Token == "" {
		logger.Panicln("bot token is nil")
	}
	logger.Debug("Read token: %s", BotConfig.Token)

	bot, err := tgbotapi.NewBotAPI(BotConfig.Token)
	if err != nil {
		logger.Panicln("Error in logging telegram: %s", err.Error())
	}
	Bot = bot
	logger.Info("Login in successful!")

	logger.Info("FullName: %s\tUserName: @%s\tUserId: %d", bot.Self.FullName(), bot.Self.UserName, bot.Self.ID)

}
