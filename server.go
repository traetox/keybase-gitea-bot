package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/keybase/go-keybase-chat-bot/kbchat"
	"github.com/keybase/go-keybase-chat-bot/kbchat/types/chat1"
	"github.com/keybase/managed-bots/base"
	"github.com/vladionescu/keybase-gitea-bot/giteabot"
	"golang.org/x/sync/errgroup"
)

const backs = "```"

type BotServer struct {
	*base.Server

	opts Options
	kbc  *kbchat.API
}

func NewBotServer(opts Options) *BotServer {
	return &BotServer{
		Server: base.NewServer("giteabot", opts.Announcement, opts.AWSOpts, opts.MultiDSN),
		opts:   opts,
	}
}

func (s *BotServer) getConfig() (webhookSecret string, err error) {
	if s.opts.WebhookSecret != "" {
		return s.opts.WebhookSecret, nil
	}
	path := fmt.Sprintf("/keybase/private/%s/credentials.json", s.kbc.GetUsername())
	cmd := s.opts.Command("fs", "read", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	s.Debug("Running `keybase fs read` on %q and waiting for it to finish...\n", path)
	if err := cmd.Run(); err != nil {
		return "", err
	}

	var j struct {
		WebhookSecret string `json:"webhook_secret"`
	}

	if err := json.Unmarshal(out.Bytes(), &j); err != nil {
		return "", err
	}

	return j.WebhookSecret, nil
}

func (s *BotServer) makeAdvertisement() kbchat.Advertisement {
	subExtended := fmt.Sprintf(`Enables posting updates from the provided Gitea project to this conversation.

Example:%s
!gitea subscribe vlad/Managed-Qubes%s`,
		backs, backs)

	unsubExtended := fmt.Sprintf(`Disables updates from the provided Gitea project to this conversation.

Example:%s
!gitea unsubscribe vlad/Report-Templates%s`,
		backs, backs)

	cmds := []chat1.UserBotCommandInput{
		{
			Name:        "gitea echo",
			Description: "Echo back the user input",
		},
		{
			Name:        "gitea subscribe",
			Description: "Enable updates from Gitea projects",
			ExtendedDescription: &chat1.UserBotExtendedDescription{
				Title:       `*!gitea subscribe* <username/project>`,
				DesktopBody: subExtended,
				MobileBody:  subExtended,
			},
		},
		{
			Name:        "gitea unsubscribe",
			Description: "Disable updates from Gitea projects",
			ExtendedDescription: &chat1.UserBotExtendedDescription{
				Title:       `*!gitea unsubscribe* <username/project>`,
				DesktopBody: unsubExtended,
				MobileBody:  unsubExtended,
			},
		},
		{
			Name:        "gitea list",
			Description: "Lists all your subscriptions.",
		},
		base.GetFeedbackCommandAdvertisement(s.kbc.GetUsername()),
	}
	return kbchat.Advertisement{
		Alias: "Gitea Bot",
		Advertisements: []chat1.AdvertiseCommandAPIParam{
			{
				Typ:      "public",
				Commands: cmds,
			},
		},
	}
}

func (s *BotServer) Go() (err error) {
	if s.kbc, err = s.Start(s.opts.KeybaseLocation, s.opts.Home, s.opts.ErrReportConv); err != nil {
		return err
	}

	secret, err := s.getConfig()
	if err != nil {
		s.Errorf("failed to get configuration: %s", err)
		return
	}

	sdb, err := sql.Open("mysql", s.opts.DSN)
	if err != nil {
		s.Errorf("failed to connect to MySQL: %s", err)
		return err
	}
	defer sdb.Close()
	db := giteabot.NewDB(sdb)

	if _, err := s.kbc.AdvertiseCommands(s.makeAdvertisement()); err != nil {
		s.Errorf("advertise error: %s", err)
		return err
	}
	if err := s.SendAnnouncement(s.opts.Announcement, "🦜 chirp. chirp."); err != nil {
		s.Errorf("failed to announce self: %s", err)
	}

	debugConfig := base.NewChatDebugOutputConfig(s.kbc, s.opts.ErrReportConv)
	stats, err := base.NewStatsRegistry(debugConfig, s.opts.StathatEZKey)
	if err != nil {
		s.Debug("unable to create stats", err)
		return err
	}
	stats = stats.SetPrefix(s.Name())

	handler := giteabot.NewHandler(stats, s.kbc, debugConfig, db, s.opts.HTTPPrefix, secret, s.opts.GiteaURL, s.opts.um, s.opts.DirectMessages)
	httpSrv := giteabot.NewHTTPSrv(stats, s.kbc, debugConfig, db, handler, secret)

	eg := &errgroup.Group{}
	s.GoWithRecover(eg, func() error { return s.Listen(handler) })
	s.GoWithRecover(eg, httpSrv.Listen)
	s.GoWithRecover(eg, func() error { return s.HandleSignals(httpSrv) })
	if err := eg.Wait(); err != nil {
		s.Debug("wait error: %s", err)
		return err
	}
	return nil
}
