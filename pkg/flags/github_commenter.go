package flags

import (
	"github.com/spf13/pflag"
)

var commentProcessingDryRunDefault = true

// GithubCommenterFlags holds configuration information for filtering Github repository.
type GithubCommenterFlags struct {
	IncludeReposCommenting  []string
	ExcludeReposCommenting  []string
	CommentProcessing       bool
	CommentProcessingDryRun bool
}

func NewGithubCommenterFlags() *GithubCommenterFlags {
	return &GithubCommenterFlags{}
}

func (f *GithubCommenterFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringArrayVar(&f.IncludeReposCommenting, "include-repo-commenting", f.IncludeReposCommenting, "Which repos do we include for pr commenting (one repo per arg instance  org/repo or just repo if openshift org)")
	fs.StringArrayVar(&f.ExcludeReposCommenting, "exclude-repo-commenting", f.ExcludeReposCommenting, "Which repos do we skip for pr commenting (one repo per arg instance  org/repo or just repo if openshift org)")
	fs.BoolVar(&f.CommentProcessing, "comment-processing", f.CommentProcessing, "Enable comment processing for github repos")
	fs.BoolVar(&f.CommentProcessingDryRun, "comment-processing-dry-run", commentProcessingDryRunDefault, "Enable github comment interaction for comment processing, disabled by default")
}
