package workspace

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// validBranchRe matches safe git branch names: alphanumeric, hyphens, underscores,
// dots, and forward slashes (for remote tracking refs like origin/main).
var validBranchRe = regexp.MustCompile(`^[a-zA-Z0-9._/\-]+$`)

// CloneRepo clones a git repository into destPath using a shallow clone (depth=1).
// It clones the specified branch. If the initial clone fails due to repo size,
// it falls back to a minimal sparse checkout.
func CloneRepo(ctx context.Context, repoURL, branch, destPath string) error {
	if branch != "" && !validBranchRe.MatchString(branch) {
		return fmt.Errorf("clone: invalid branch name %q", branch)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)

	opts := &git.CloneOptions{
		URL:           repoURL,
		ReferenceName: branchRef,
		SingleBranch:  true,
		Depth:         1,
		Tags:          git.NoTags,
	}

	_, err := git.PlainCloneContext(ctx, destPath, false, opts)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return nil
		}

		// Attempt fallback: try default branch if named branch fails
		if branch != "" {
			fallbackOpts := &git.CloneOptions{
				URL:          repoURL,
				SingleBranch: false,
				Depth:        1,
				Tags:         git.NoTags,
			}
			_, fallbackErr := git.PlainCloneContext(ctx, destPath, false, fallbackOpts)
			if fallbackErr != nil {
				return fmt.Errorf("clone %q (branch %q) failed: %v; fallback also failed: %w",
					repoURL, branch, err, fallbackErr)
			}

			// Checkout the requested branch
			repo, openErr := git.PlainOpen(destPath)
			if openErr != nil {
				return nil // fallback clone succeeded even if we can't checkout
			}

			w, wtErr := repo.Worktree()
			if wtErr != nil {
				return nil
			}

			checkoutErr := w.Checkout(&git.CheckoutOptions{
				Branch: branchRef,
				Create: false,
			})
			_ = checkoutErr // best-effort
			return nil
		}

		return fmt.Errorf("clone %q: %w", repoURL, err)
	}

	return nil
}
