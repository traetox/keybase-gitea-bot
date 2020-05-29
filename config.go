package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gravwell/gcfg"
	"github.com/keybase/managed-bots/base"
)

const version = "1.0.0"

type Options struct {
	*base.Options
	um             map[string]string
	HTTPPrefix     string
	WebhookSecret  string
	GiteaURL       string
	DirectMessages bool
}

type userMaps varconfig

type varconfig struct {
	gcfg.Idxer
	Vals map[gcfg.Idx]*[]string
}

func LoadOptions() (opts *Options, ec int, ok bool) {
	var cfgFile string
	opts = &Options{
		Options: base.NewOptions(),
	}

	//load up any options that are coming in from flags
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&cfgFile, "config-file", ``, "Path to configuration file")
	fs.StringVar(&opts.HTTPPrefix, "http-prefix", os.Getenv("BOT_HTTP_PREFIX"), "host:port of bot's HTTP server listening for incoming webhooks")
	fs.StringVar(&opts.WebhookSecret, "secret", os.Getenv("BOT_WEBHOOK_SECRET"), "Webhook secret")
	fs.StringVar(&opts.GiteaURL, "gitea-url", os.Getenv("BOT_GITEA_URL"), "URL of the Gitea server, for pretty links in announcements")
	fs.BoolVar(&opts.DirectMessages, "direct-messages", false, "Send messages directly to user when username specified")
	showVersion := fs.Bool("version", false, "display the version and quit")
	if *showVersion {
		fmt.Printf("giteabot v%s\n", version)
		return
	}

	if err := opts.Parse(fs, os.Args); err != nil {
		fmt.Printf("Unable to parse options: %v\n", err)
		ec = 3
		return
	}
	if cfgFile != `` {
		cf := struct {
			BotConfig Options
			UserMaps  userMaps
		}{
			BotConfig: *opts,
		}
		if err := gcfg.ReadFileInto(&cf, cfgFile); err != nil {
			fmt.Printf("Failed to read config file: %v\n", err)
			ec = 3
			return
		}
		*opts = cf.BotConfig
		if mp, err := cf.UserMaps.usernames(); err != nil {
			fmt.Printf("Invalid usernames: %v\n", err)
			ec = 3
			return
		} else if len(mp) > 0 {
			opts.um = mp
		}
	}
	if len(opts.DSN) == 0 {
		fmt.Printf("must specify a database DSN\n\n")
		fs.PrintDefaults()
		ec = 3
		return
	}

	ok = true
	return
}

func (vc userMaps) usernames() (mp map[string]string, err error) {
	var tmp *[]string
	mp = make(map[string]string, len(vc.Idxer.Names()))
	for _, k := range vc.Idxer.Names() {
		if tmp = vc.Vals[vc.Idx(k)]; tmp == nil || len(*tmp) == 0 || (*tmp)[0] == `` {
			err = fmt.Errorf("Username override %s is empty", k)
			return
		} else if len(*tmp) > 1 {
			err = fmt.Errorf("Username override %s is repeated", k)
			return
		} else {
			mp[k] = (*tmp)[0]
		}
	}
	return
}
