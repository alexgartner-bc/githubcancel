package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/bep/debounce"
	"github.com/cbrgm/githubevents/githubevents"
	"github.com/google/go-github/v50/github"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

var client *github.Client

var owner, repo string

var logger *zap.Logger

func init() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(err)
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := &http.Client{
		Transport: &oauth2.Transport{
			Source: ts,
		},
	}
	client = github.NewClient(tc)
	owner = os.Getenv("GITHUB_OWNER")
	repo = os.Getenv("GITHUB_REPO")
}

func cancelExtraWorkflows() {
	ctx := context.Background()
	res, _, err := client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &github.ListWorkflowRunsOptions{
		Event: githubevents.PullRequestEvent,
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
	})
	if err != nil {
		logger.Error("unable to get workflow runs", zap.Error(err))
		return
	}
	runs := res.WorkflowRuns
	runs = lo.Filter(runs, func(run *github.WorkflowRun, _ int) bool {
		return *run.Status != githubevents.WorkflowRunEventCompletedAction
	})
	groupedRuns := lo.GroupBy(runs, func(run *github.WorkflowRun) string {
		return fmt.Sprintf("%d-%s", *run.WorkflowID, *run.HeadBranch)
	})
	allWorkflowIdsToCancel := []int64{}
	for groupKey, group := range groupedRuns {
		if len(group) == 1 {
			continue
		}
		// run number seems more ordered than runId?
		sort.Slice(group, func(i, j int) bool {
			return *group[i].RunNumber > *group[j].RunNumber
		})
		workflowsToCancel := group[1:]
		workflowIdsToCancel := lo.Map(workflowsToCancel, func(run *github.WorkflowRun, _ int) int64 {
			return *run.ID
		})
		logger.Info("canceling workflows because a more recent one exists",
			zap.String("groupKey", groupKey),
			zap.Int("len", len(workflowsToCancel)),
			zap.Int64("keepId", *group[0].ID),
			zap.Int64s("cancelIds", workflowIdsToCancel),
		)
		allWorkflowIdsToCancel = append(workflowIdsToCancel, allWorkflowIdsToCancel...)
	}

	// do not run in parallel to avoid rate limits
	// this cancellation is best effort only
	for _, id := range allWorkflowIdsToCancel {
		_, err = client.Actions.CancelWorkflowRunByID(ctx, owner, repo, id)
		if err != nil {
			logger.Warn("unable to cancel run", zap.Int64("id", id), zap.Error(err))
		}
	}
}

func main() {
	handle := githubevents.New("")

	debouncer := debounce.New(time.Second * 2)

	handle.OnWorkflowRunEventRequested(func(deliveryID, eventName string, event *github.WorkflowRunEvent) error {
		if event.WorkflowRun == nil {
			logger.Warn("nil workflowrun")
			return nil
		}
		if *event.WorkflowRun.Event != githubevents.PullRequestEvent {
			return nil
		}
		debouncer(cancelExtraWorkflows)
		return nil
	})

	http.HandleFunc("/hook", func(w http.ResponseWriter, r *http.Request) {
		err := handle.HandleEventRequest(r)
		if err != nil {
			logger.Error("unable to handle request", zap.Error(err))
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		panic(err)
	}
}
