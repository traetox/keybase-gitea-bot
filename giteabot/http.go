package giteabot

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	gitea "code.gitea.io/gitea/modules/structs"
	"github.com/keybase/go-keybase-chat-bot/kbchat"
	"github.com/keybase/managed-bots/base"
)

type HTTPSrv struct {
	*base.HTTPSrv

	kbc     *kbchat.API
	db      *DB
	handler *Handler
	secret  string
}

func NewHTTPSrv(stats *base.StatsRegistry, kbc *kbchat.API, debugConfig *base.ChatDebugOutputConfig,
	db *DB, handler *Handler, secret string) *HTTPSrv {
	h := &HTTPSrv{
		kbc:     kbc,
		db:      db,
		handler: handler,
		secret:  secret,
	}
	h.HTTPSrv = base.NewHTTPSrv(stats, debugConfig)
	http.HandleFunc("/giteabot", h.handleHealthCheck)
	http.HandleFunc("/giteabot/webhook", h.handleWebhook)
	return h
}

func (h *HTTPSrv) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "beep boop! :)")
}

func (h *HTTPSrv) handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.Errorf("Error reading payload: %s", err)
		return
	}
	defer r.Body.Close()

	event, err := ParseWebhook(WebhookEventType(r), payload)
	if err != nil {
		h.Errorf("could not parse webhook: type:%v %s\n", WebhookEventType(r), err)
		return
	}

	var message, repo, secret string

	// Event types are defined in gitea/modules/structs/hook.go as xxxxPayload
	//   https://github.com/go-gitea/gitea/blob/master/modules/structs/hook.go
	switch event := event.(type) {
	case *gitea.PushPayload:
		// Gitea will send a bogus "push" event when a release is created
		// Ignore these, since they're not real commits/pushes
		if len(event.Commits) == 0 {
			return
		}

		message = FormatPushMsg(
			event.Pusher.FullName,
			event.Repo.FullName,
			refToBranch(event.Ref),
			len(event.Commits),
			getCommitMessages(event),
			event.Commits[len(event.Commits)-1].URL,
		)

		repo = event.Repo.FullName
		secret = event.Secret
	case *gitea.CreatePayload:
		message = FormatCreateMsg(
			event.Ref,
			event.RefType,
			event.Repo.FullName,
		)

		repo = event.Repo.FullName
		secret = event.Secret
	case *gitea.DeletePayload:
		message = FormatDeleteMsg(
			event.Ref,
			event.RefType,
			event.Repo.FullName,
		)

		repo = event.Repo.FullName
		secret = event.Secret
	case *gitea.ForkPayload:
		message = FormatForkMsg(
			event.Forkee.FullName,
			event.Repo.FullName,
		)

		repo = event.Forkee.FullName
		secret = event.Secret
	case *gitea.IssuePayload:
		assignee := getAssignees(event.Issue.Assignee, event.Issue.Assignees)
		message = FormatIssueMsg(
			event.Action,
			event.Sender.FullName,
			event.Issue.Index,
			event.Repository.FullName,
			assignee,
			event.Issue.Title,
			event.Issue.HTMLURL,
		)
		if h.handler.dmUsers {
			handled, err := h.handleDMs(
				event.Issue.Poster,
				event.Issue.Assignee,
				event.Issue.Assignees,
				message)
			if err != nil {
				h.Errorf("Failed to handle DMS for repo %s: %v",
					event.Repository.FullName, err)
				return
			} else if handled {
				return
			}
		}

		repo = event.Repository.FullName
		secret = event.Secret
	case *gitea.IssueCommentPayload:
		message = FormatIssueCommentMsg(
			event.Action,
			event.Comment.Poster.FullName,
			event.Issue.Index,
			event.Repository.FullName,
			event.Comment.Body,
			event.Issue.Title,
			event.Comment.HTMLURL,
		)
		if h.handler.dmUsers {
			handled, err := h.handleDMs(
				event.Comment.Poster,
				event.Issue.Assignee,
				event.Issue.Assignees,
				message)
			if err != nil {
				h.Errorf("Failed to handle DMS for repo %s: %v",
					event.Repository.FullName, err)
				return
			} else if handled {
				return
			}
		}

		repo = event.Repository.FullName
		secret = event.Secret
	case *gitea.RepositoryPayload:
		message = FormatRepositoryMsg(
			event.Action,
			event.Sender.FullName,
			event.Repository.FullName,
		)

		repo = event.Repository.FullName
		secret = event.Secret
	case *gitea.ReleasePayload:
		message = FormatReleaseMsg(
			event.Action,
			event.Sender.FullName,
			event.Repository.FullName,
			event.Release.Title,
			event.Release.TagName,
			event.Release.TarURL,
		)

		repo = event.Repository.FullName
		secret = event.Secret
	case *gitea.PullRequestPayload:
		assignee := getAssignees(event.PullRequest.Assignee, event.PullRequest.Assignees)

		source := fmt.Sprintf("%s/%s", event.PullRequest.Head.Repository.FullName, event.PullRequest.Head.Name)

		message = FormatPullRequestMsg(
			event.Action,
			event.Sender.FullName,
			event.Repository.FullName,
			event.PullRequest.Index,
			event.PullRequest.Title,
			source,
			assignee,
			event.PullRequest.HTMLURL,
		)
		if h.handler.dmUsers {
			handled, err := h.handleDMs(
				event.PullRequest.Poster,
				event.PullRequest.Assignee,
				event.PullRequest.Assignees,
				message)
			if err != nil {
				h.Errorf("Failed to handle DMS for repo %s: %v",
					event.Repository.FullName, err)
				return
			} else if handled {
				return
			}
		}

		repo = event.Repository.FullName
		secret = event.Secret
	}

	if message == "" || repo == "" {
		return
	}

	repo = strings.ToLower(repo)
	convs, err := h.db.GetSubscribedConvs(repo)
	if err != nil {
		h.Errorf("Error getting subscriptions for repo: %s", err)
		return
	}

	for _, convID := range convs {
		var secretToken = base.MakeSecret(repo, convID, h.secret)
		if secret != secretToken {
			h.Debug("Error validating payload signature for conversation %s: %v", convID, err)
			continue
		}
		h.ChatEcho(convID, message)
	}
}

// handleDMs will send chat messages directly to assignees if we can resolve the keybase names
// return values are handled and error
//  handled means we got all messages off to all assignees
// we will NOT DM users if they are the origininator of the gitea event
func (h *HTTPSrv) handleDMs(sender, assignee *gitea.User, assignees []*gitea.User, msg string, args ...interface{}) (handled bool, err error) {
	var ok bool
	if sender == nil || (assignee == nil && len(assignees) == 0) || len(h.handler.users) == 0 {
		return //just ignore it, go the usual route
	}
	handled = true //we assume success
	if assignee != nil && sender.ID != assignee.ID {
		if ok, err = h.handleDM(assignee.UserName, msg, args...); err != nil {
			return
		} else if !ok {
			handled = false
		}
	}
	for _, u := range assignees {
		//check against sender and assignee
		//sender because we don't want to notify on our own actions, and assignee so we don't fire twice
		if u == nil || u.ID == sender.ID || (assignee != nil && assignee.ID == u.ID) {
			continue
		}
		if ok, err = h.handleDM(u.UserName, msg, args...); err != nil {
			return
		} else if !ok {
			handled = false
		}
	}
	return
}

func (h *HTTPSrv) handleDM(user, msg string, args ...interface{}) (ok bool, err error) {
	var kbuser string
	//try to lookup the keybase user
	if kbuser, ok = h.handler.users[user]; ok {
		err = h.handler.ChatUser(kbuser, msg, args...)
	}
	return
}
