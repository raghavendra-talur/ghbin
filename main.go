package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/google/go-github/v33/github"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
)

func main() {
	app := &cli.App{
		Name:  "ghbin",
		Usage: "Turn a Github repo into a pastebin",
		Commands: []*cli.Command{
			{
				Name:    "upload",
				Aliases: []string{"u"},
				Usage:   "Upload a file or clipboard content to Github",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "path",
						Aliases: []string{"p"},
						Usage:   "Path(s) to file(s) to upload",
					},
					&cli.BoolFlag{
						Name:    "clipboard",
						Aliases: []string{"x"},
						Usage:   "Upload content from clipboard",
					},
					&cli.StringFlag{
						Name:    "file-name",
						Aliases: []string{"f"},
						Usage:   "Specify a filename for clipboard content",
					},
					&cli.StringFlag{
						Name:    "message",
						Aliases: []string{"m"},
						Usage:   "Commit message",
					},
					&cli.StringFlag{
						Name:    "target-dir",
						Aliases: []string{"d"},
						Usage:   "Target directory in the repo",
					},
					&cli.BoolFlag{
						Name:  "new",
						Usage: "Create a new file if it already exists",
					},
				},
				Action: uploadAction,
			},
			{
				Name:    "download",
				Aliases: []string{"dl"},
				Usage:   "Download a file or directory from Github",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "path",
						Aliases:  []string{"p"},
						Usage:    "Path to file or directory to download",
						Required: true,
					},
				},
				Action: downloadAction,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func getClipboardContent() ([]byte, error) {
	// Read string from clipboard
	text, err := clipboard.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read from clipboard: %w", err)
	}

	// If the clipboard is empty, return an error
	if text == "" {
		return nil, fmt.Errorf("clipboard is empty")
	}

	// Convert string to []byte
	return []byte(text), nil
}

func uploadAction(c *cli.Context) error {
	token := os.Getenv("GHBIN_GITHUB_TOKEN")
	repoName := os.Getenv("GHBIN_REPO")
	if token == "" || repoName == "" {
		return fmt.Errorf("GHBIN_GITHUB_TOKEN and GHBIN_REPO environment variables must be set")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	paths := c.StringSlice("path")
	fromClipboard := c.Bool("clipboard")
	fileName := c.String("file-name")
	message := c.String("message")
	targetDir := c.String("target-dir")
	forceNew := c.Bool("new")

	if fromClipboard {
		content, err := getClipboardContent()
		if err != nil {
			return err
		}

		if fileName == "" {
			fileName, err = generateRandomFileName()
			if err != nil {
				return err
			}
		}

		err = uploadContent(ctx, client, repoName, fileName, content, message, targetDir, forceNew)
		if err != nil {
			return err
		}
	} else {
		if len(paths) == 0 {
			return fmt.Errorf("at least one path must be provided")
		}

		for _, path := range paths {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fileName := filepath.Base(path)
			err = uploadContent(ctx, client, repoName, fileName, content, message, targetDir, forceNew)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func uploadContent(ctx context.Context, client *github.Client, repoName, fileName string, content []byte, message, targetDir string, forceNew bool) error {
	owner, repo, err := parseRepoName(repoName)
	if err != nil {
		return err
	}

	path := filepath.Join(targetDir, fileName)

	// Check if file exists
	_, _, _, err = client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err == nil {
		if forceNew {
			fileName, err = generateRandomFileName()
			if err != nil {
				return err
			}

			path = filepath.Join(targetDir, fileName)
		} else {
			// Update existing file
			currentFile, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
			if err != nil {
				return err
			}
			_, _, err = client.Repositories.UpdateFile(ctx, owner, repo, path, &github.RepositoryContentFileOptions{
				Message: github.String(message),
				Content: content,
				SHA:     currentFile.SHA,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Updated file: %s\n", path)
			return nil
		}
	}

	// Create new file
	_, _, err = client.Repositories.CreateFile(ctx, owner, repo, path, &github.RepositoryContentFileOptions{
		Message: github.String(message),
		Content: content,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Created file: %s\n", path)
	return nil
}

func parseRepoName(repoName string) (string, string, error) {
	parts := strings.Split(repoName, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo name format. Expected 'owner/repo'")
	}
	return parts[0], parts[1], nil
}

func generateRandomFileName() (string, error) {
	b := make([]byte, 6)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random file name: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b) + ".txt", nil
}

func downloadAction(c *cli.Context) error {
	token := os.Getenv("GHBIN_GITHUB_TOKEN")
	repoName := os.Getenv("GHBIN_REPO")
	if token == "" || repoName == "" {
		return fmt.Errorf("GHBIN_GITHUB_TOKEN and GHBIN_REPO environment variables must be set")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	path := c.String("path")
	owner, repo, err := parseRepoName(repoName)
	if err != nil {
		return err
	}

	return downloadContent(ctx, client, owner, repo, path)
}

func downloadContent(ctx context.Context, client *github.Client, owner, repo, path string) error {
	content, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return err
	}

	if content.GetType() == "file" {
		return downloadFile(content)
	} else if content.GetType() == "dir" {
		return downloadDirectory(ctx, client, owner, repo, path)
	}

	return fmt.Errorf("unknown content type: %s", content.GetType())
}

func downloadFile(content *github.RepositoryContent) error {
	decodedContent, err := content.GetContent()
	if err != nil {
		return err
	}

	err = os.WriteFile(content.GetName(), []byte(decodedContent), 0o644)
	if err != nil {
		return err
	}

	fmt.Printf("Downloaded file: %s\n", content.GetName())
	return nil
}

func downloadDirectory(ctx context.Context, client *github.Client, owner, repo, path string) error {
	_, directoryContent, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return err
	}

	for _, item := range directoryContent {
		if item.GetType() == "file" {
			err = downloadFile(item)
			if err != nil {
				return err
			}
		} else if item.GetType() == "dir" {
			err = os.MkdirAll(item.GetName(), 0o755)
			if err != nil {
				return err
			}
			err = downloadDirectory(ctx, client, owner, repo, item.GetPath())
			if err != nil {
				return err
			}
		}
	}

	fmt.Printf("Downloaded directory: %s\n", path)
	return nil
}
