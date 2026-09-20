package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/bot"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/logging"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/privdrop"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/server"
)

// defaultContainerUID/GID match the UID/GID the Docker image ran as before
// it started dropping privileges at runtime, so an operator who sets
// neither PUID nor PGID sees the same effective permissions as before.
const (
	defaultContainerUID = 65532
	defaultContainerGID = 65532
)

var cfgFile string

// Version is the build version, set via -ldflags "-X ...cmd.Version=..." at
// release time.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:           "peace-breaker-bot",
	Short:         "Application layer that manages the bot and creates an HTTP inteface for controlling the bot playback",
	Version:       Version,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runRootCmd,
}

// Execute runs the root command, exiting the process on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.peace-breaker-bot.yaml)")

	rootCmd.PersistentFlags().String("bot-owner", "", "bot owner username")
	viper.BindPFlag("bot.owner", rootCmd.PersistentFlags().Lookup("bot-owner"))

	rootCmd.PersistentFlags().String("bot-token", "", "application oauth token to authenticate the bot")
	viper.BindPFlag("bot.token", rootCmd.PersistentFlags().Lookup("bot-token"))

	rootCmd.PersistentFlags().String("server-address", "", "address to bind the http server")
	viper.BindPFlag("server.address", rootCmd.PersistentFlags().Lookup("server-address"))

	rootCmd.PersistentFlags().String("bot-locale", "", "fixes the bot's response language (e.g. en-US, pt-BR); defaults to the invoking guild's own locale")
	viper.BindPFlag("bot.locale", rootCmd.PersistentFlags().Lookup("bot-locale"))

	rootCmd.PersistentFlags().String("log-level", "", "log verbosity: debug, info, warn or error (default info)")
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
}

func runRootCmd(cmd *cobra.Command, args []string) error {
	// initConfig can't fail a command, so a bad level is only applied there
	// when valid; it is rejected here, before anything starts.
	if _, err := logging.ParseLevel(viper.GetString("log.level")); err != nil {
		return err
	}

	if err := dropPrivileges(); err != nil {
		return fmt.Errorf("failed to drop privileges: %w", err)
	}

	token := viper.GetString("bot.token")
	if strings.TrimSpace(token) == "" {
		return errors.New("bot token not provided")
	}

	owner := viper.GetString("bot.owner")
	if strings.TrimSpace(owner) == "" {
		return errors.New("bot owner not provided")
	}

	address := viper.GetString("server.address")
	if strings.TrimSpace(address) == "" {
		return errors.New("server address not provided")
	}

	locale := viper.GetString("bot.locale")

	slog.Debug("config resolved",
		"logLevel", viper.GetString("log.level"),
		"configFile", viper.ConfigFileUsed(),
		"owner", owner,
		"address", address,
		"locale", locale,
		"tokenSet", true,
	)

	errchan := make(chan error, 1)
	defer close(errchan)

	player := instant.NewPlayer()
	defer player.Close()

	b, err := bot.New(token, player, bot.WithOwner(owner), bot.WithLocale(locale), bot.WithVersion(Version))
	if err != nil {
		return fmt.Errorf("failed to create a bot: %w", err)
	}

	go func() {
		if err := b.Start(); err != nil {
			errchan <- err
		}
	}()

	s := server.New(player, b)

	go func() {
		if err := s.Start(address); err != nil {
			errchan <- err
		}
	}()

	err = <-errchan

	slog.Error("bot exited", "err", err)
	time.Sleep(time.Second * 3)

	return nil
}

// dropPrivileges chowns the instant cache dir to PUID/PGID (both read
// straight from the environment, not through Viper, since they're Docker
// plumbing rather than app config) and permanently drops the process to
// that uid/gid. It's a no-op outside a root-started container — see
// pkg/privdrop.
func dropPrivileges() error {
	dir, err := fsutil.GetCacheDirOrCreate()
	if err != nil {
		return fmt.Errorf("failed to prepare cache dir: %w", err)
	}

	uid, err := envIntOrDefault("PUID", defaultContainerUID)
	if err != nil {
		return err
	}

	gid, err := envIntOrDefault("PGID", defaultContainerGID)
	if err != nil {
		return err
	}

	dropped, err := privdrop.DropTo(dir, uid, gid)
	if err != nil {
		return err
	}
	if dropped {
		slog.Info("dropped privileges", "uid", uid, "gid", gid, "dir", dir)
	} else {
		slog.Debug("privileges not dropped", "euid", os.Geteuid(), "cacheDir", dir)
	}

	return nil
}

func envIntOrDefault(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return n, nil
}

func initConfig() {
	logging.Setup(os.Stdout)

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Error("find homedir", "err", err)
			os.Exit(1)
		}

		cwd, err := os.Getwd()
		if err != nil {
			slog.Error("find cwd", "err", err)
			os.Exit(1)
		}

		viper.AddConfigPath(home)
		viper.AddConfigPath(cwd)
		viper.SetConfigName(".peace-breaker-bot")
	}

	replacer := strings.NewReplacer(
		".", "_",
		"-", "_",
	)
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()

	readErr := viper.ReadInConfig()

	// Applied before the line below so a quieter level also silences it. An
	// invalid value keeps the default here and is rejected by runRootCmd.
	if level, err := logging.ParseLevel(viper.GetString("log.level")); err == nil {
		logging.SetLevel(level)
	}

	if readErr == nil {
		slog.Info("using config file", "configFile", viper.ConfigFileUsed())
	}
}
