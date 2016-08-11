package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bradfitz/issuemirror"
	"github.com/google/go-github/github"
	jekyllissues "github.com/parkr/jekyll-issue-mirror/issues"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
)

const (
	owner = "jekyll"
	repo  = "jekyll"
)

func writeIssues(client *github.Client, root issuemirror.Root, issues []*github.Issue) error {
	g, _ := errgroup.WithContext(context.Background())
	log.Printf("processing %d issues", len(issues))
	for _, issue := range issues {
		issueVal := *issue
		num := *issueVal.Number
		g.Go(func() error {
			start := time.Now()
			log.Printf("started processing %d at %v", num, start)
			// Write issue
			issueFile := root.IssueJSONFile(num)
			err := os.MkdirAll(filepath.Dir(issueFile), 0755)
			if err != nil {
				return err
			}
			issueJSON, err := json.Marshal(issueVal)
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(issueFile, issueJSON, 0644)
			if err != nil {
				return err
			}

			// Are there comments?
			if *issue.Comments <= 0 {
				return nil
			}

			// OK, now handle the comments.
			commentsDir := root.IssueCommentsDir(num)
			err = os.MkdirAll(commentsDir, 0755)
			if err != nil {
				return err
			}
			opt := &github.IssueListCommentsOptions{
				Sort:      "created",
				Direction: "asc",
				ListOptions: github.ListOptions{
					Page:    0,
					PerPage: 100,
				},
			}
			for {
				log.Printf("client.Issues.ListComments(%s, %s, %d, %s)", owner, repo, num, github.Stringify(opt))
				comments, resp, err := client.Issues.ListComments(owner, repo, num, opt)
				if err != nil {
					log.Fatalf("listing comments for issue=%d; page %d: %v", num, opt.ListOptions.Page, err)
				}
				err = writeComments(root, issueVal, comments)
				if err != nil {
					log.Fatalf("writing comments for issue=%d; page %d: %v", num, opt.ListOptions.Page, err)
				}
				if resp.NextPage == 0 {
					break
				}
				opt.ListOptions.Page = resp.NextPage
			}
			log.Printf("finished processing %d in %s", num, time.Since(start))
			return nil
		})
	}
	return g.Wait()
}

func writeComments(root issuemirror.Root, issue github.Issue, comments []*github.IssueComment) error {
	g, _ := errgroup.WithContext(context.Background())
	log.Printf("processing %d comments for issue=%d", len(comments), *issue.Number)
	for _, comment := range comments {
		commentVal := *comment
		g.Go(func() error {
			commentFile := root.IssueCommentFile(*issue.Number, *commentVal.ID)
			commentJSON, err := json.Marshal(commentVal)
			if err != nil {
				return err
			}
			return ioutil.WriteFile(commentFile, commentJSON, 0644)
		})
	}
	return g.Wait()
}

func main() {
	root, err := jekyllissues.Open()
	if err != nil {
		log.Fatalln("error opening issue cache folder", err)
	}

	client := github.NewClient(oauth2.NewClient(
		oauth2.NoContext,
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		),
	))
	opt := &github.IssueListByRepoOptions{
		State:     "open",
		Sort:      "created",
		Direction: "asc",
		ListOptions: github.ListOptions{
			Page:    0,
			PerPage: 100,
		},
	}

	for {
		log.Printf("client.Issues.ListByRepo(%s, %s, %s)", owner, repo, github.Stringify(opt))
		issues, resp, err := client.Issues.ListByRepo(owner, repo, opt)
		if err != nil {
			log.Fatalln("listing issues; page", opt.ListOptions.Page, err)
		}
		err = writeIssues(client, root, issues)
		if err != nil {
			log.Fatalln("writing issues; page", opt.ListOptions.Page, err)
		}
		if resp.NextPage == 0 {
			log.Println("no more pages")
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}
}
