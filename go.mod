module github.com/vladionescu/keybase-gitea-bot

go 1.13

require (
	code.gitea.io/gitea v1.11.5
	github.com/go-sql-driver/mysql v1.5.0
	github.com/gravwell/gcfg v1.2.5
	github.com/keybase/go-keybase-chat-bot v0.0.0-20200207200343-9aca502dc88a
	github.com/keybase/managed-bots v0.0.0-20200213191121-9c4cb4f69664
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	gopkg.in/gcfg.v1 v1.2.3 // indirect
)

replace github.com/vladionescu/keybase-gitea-bot/giteabot => ./giteabot
