package updater

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gitops-tools/image-hooks/pkg/client/mock"
	"github.com/gitops-tools/image-hooks/pkg/config"
	"github.com/gitops-tools/image-hooks/pkg/hooks/quay"
	"github.com/jenkins-x/go-scm/scm"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

const (
	testQuayRepo   = "mynamespace/repository"
	testGitHubRepo = "testorg/testrepo"
	testFilePath   = "environments/test/services/service-a/test.yaml"
)

func TestUpdaterWithUnknownRepo(t *testing.T) {
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, "master", []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, "master", testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()

	updater := New(logger, m, createConfigs())
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()
	hook.Repository = "unknown/repo"

	err := updater.UpdateFromHook(context.Background(), hook)

	// A non-matching repo is not considered an error.
	if err != nil {
		t.Fatal(err)
	}
	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, "test-branch-a")
	if s := string(updated); s != "" {
		t.Fatalf("update failed, got %#v, want %#v", s, "")
	}
	m.RefuteBranchCreated(testGitHubRepo, "test-branch-a", testSHA)
	m.RefutePullRequestCreated(testGitHubRepo, &scm.PullRequestInput{
		Title: fmt.Sprintf("Image %s updated", testQuayRepo),
		Body:  "Automated Image Update",
		Head:  "test-branch-a",
		Base:  "master",
	})
}

func TestUpdaterWithKnownRepo(t *testing.T) {
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, "master", []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, "master", testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()

	updater := New(logger, m, createConfigs())
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()

	err := updater.UpdateFromHook(context.Background(), hook)
	if err != nil {
		t.Fatal(err)
	}

	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, "test-branch-a")
	want := "test:\n  image: quay.io/testorg/repo:production\n"
	if s := string(updated); s != want {
		t.Fatalf("update failed, got %#v, want %#v", s, want)
	}
	m.AssertBranchCreated(testGitHubRepo, "test-branch-a", testSHA)
	m.AssertPullRequestCreated(testGitHubRepo, &scm.PullRequestInput{
		Title: fmt.Sprintf("Image %s updated", testQuayRepo),
		Body:  "Automated Image Update",
		Head:  "test-branch-a",
		Base:  "master",
	})
}

// With no name-generator, the change should be made to master directly, rather
// than going through a PullRequest.
func TestUpdaterWithNoNameGenerator(t *testing.T) {
	sourceBranch := "production"
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, sourceBranch, []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, sourceBranch, testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()

	configs := createConfigs()
	configs.Repositories[0].BranchGenerateName = ""
	configs.Repositories[0].SourceBranch = sourceBranch
	updater := New(logger, m, configs)
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()

	err := updater.UpdateFromHook(context.Background(), hook)
	if err != nil {
		t.Fatal(err)
	}

	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, sourceBranch)
	want := "test:\n  image: quay.io/testorg/repo:production\n"
	if s := string(updated); s != want {
		t.Fatalf("update failed, got %#v, want %#v", s, want)
	}
	m.AssertNoBranchesCreated()
	m.AssertNoPullRequestsCreated()
}

func TestUpdaterWithMissingFile(t *testing.T) {
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, "master", []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, "master", testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()

	updater := New(logger, m, createConfigs())
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()
	testErr := errors.New("missing file")
	m.GetFileErr = testErr

	err := updater.UpdateFromHook(context.Background(), hook)

	if err != testErr {
		t.Fatalf("got %s, want %s", err, testErr)
	}
	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, "test-branch-a")
	if s := string(updated); s != "" {
		t.Fatalf("update failed, got %#v, want %#v", s, "")
	}
	m.AssertNoBranchesCreated()
	m.AssertNoPullRequestsCreated()
}

func TestUpdaterWithBranchCreationFailure(t *testing.T) {
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, "master", []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, "master", testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()

	updater := New(logger, m, createConfigs())
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()
	testErr := errors.New("can't create branch")
	m.CreateBranchErr = testErr

	err := updater.UpdateFromHook(context.Background(), hook)

	if err.Error() != "failed to create branch: can't create branch" {
		t.Fatalf("got %s, want %s", err, "failed to create branch: can't create branch")
	}
	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, "test-branch-a")
	if s := string(updated); s != "" {
		t.Fatalf("update failed, got %#v, want %#v", s, "")
	}
	m.AssertNoBranchesCreated()
	m.AssertNoPullRequestsCreated()
}

func TestUpdaterWithUpdateFileFailure(t *testing.T) {
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, "master", []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, "master", testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()

	updater := New(logger, m, createConfigs())
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()
	testErr := errors.New("can't update file")
	m.UpdateFileErr = testErr

	err := updater.UpdateFromHook(context.Background(), hook)

	if err.Error() != "failed to update file: can't update file" {
		t.Fatalf("got %s, want %s", err, "failed to update file: can't update file")
	}
	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, "test-branch-a")
	if s := string(updated); s != "" {
		t.Fatalf("update failed, got %#v, want %#v", s, "")
	}
	m.AssertBranchCreated(testGitHubRepo, "test-branch-a", testSHA)
	m.RefutePullRequestCreated(testGitHubRepo, &scm.PullRequestInput{
		Title: fmt.Sprintf("Image %s updated", testQuayRepo),
		Body:  "Automated Image Update",
		Head:  "test-branch-a",
		Base:  "master",
	})
}

func TestUpdaterWithCreatePullRequestFailure(t *testing.T) {
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, "master", []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, "master", testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()

	updater := New(logger, m, createConfigs())
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()
	testErr := errors.New("can't create pull-request")
	m.CreatePullRequestErr = testErr

	err := updater.UpdateFromHook(context.Background(), hook)

	if err.Error() != "failed to create a pull request: can't create pull-request" {
		t.Fatalf("got %s, want %s", err, "failed to create a pull request: can't create pull-request")
	}
	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, "test-branch-a")
	want := "test:\n  image: quay.io/testorg/repo:production\n"
	if s := string(updated); s != want {
		t.Fatalf("update failed, got %#v, want %#v", s, "")
	}
	m.AssertBranchCreated(testGitHubRepo, "test-branch-a", testSHA)
	m.RefutePullRequestCreated(testGitHubRepo, &scm.PullRequestInput{
		Title: fmt.Sprintf("Image %s updated", testQuayRepo),
		Body:  "Automated Image Update",
		Head:  "test-branch-a",
		Base:  "master",
	})
}

func TestUpdaterWithNonMasterSourceBranch(t *testing.T) {
	testSHA := "980a0d5f19a64b4b30a87d4206aade58726b60e3"
	m := mock.New(t)
	m.AddFileContents(testGitHubRepo, testFilePath, "staging", []byte("test:\n  image: old-image\n"))
	m.AddBranchHead(testGitHubRepo, "staging", testSHA)
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)).Sugar()
	configs := createConfigs()
	configs.Repositories[0].SourceBranch = "staging"
	updater := New(logger, m, configs)
	updater.nameGenerator = stubNameGenerator{"a"}
	hook := createHook()

	err := updater.UpdateFromHook(context.Background(), hook)
	if err != nil {
		t.Fatal(err)
	}

	updated := m.GetUpdatedContents(testGitHubRepo, testFilePath, "test-branch-a")
	want := "test:\n  image: quay.io/testorg/repo:production\n"
	if s := string(updated); s != want {
		t.Fatalf("update failed, got %#v, want %#v", s, want)
	}
	m.AssertBranchCreated(testGitHubRepo, "test-branch-a", testSHA)
	m.AssertPullRequestCreated(testGitHubRepo, &scm.PullRequestInput{
		Title: fmt.Sprintf("Image %s updated", testQuayRepo),
		Body:  "Automated Image Update",
		Head:  "test-branch-a",
		Base:  "staging",
	})
}

func createHook() *quay.RepositoryPushHook {
	return &quay.RepositoryPushHook{
		Repository:  testQuayRepo,
		DockerURL:   "quay.io/testorg/repo",
		UpdatedTags: []string{"production"},
	}
}

func createConfigs() *config.RepoConfiguration {
	return &config.RepoConfiguration{
		Repositories: []*config.Repository{
			{
				Name:               testQuayRepo,
				SourceRepo:         testGitHubRepo,
				SourceBranch:       "master",
				FilePath:           testFilePath,
				UpdateKey:          "test.image",
				BranchGenerateName: "test-branch-",
			},
		},
	}
}

type stubNameGenerator struct {
	name string
}

func (s stubNameGenerator) PrefixedName(p string) string {
	return p + s.name
}
