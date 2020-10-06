package tektonlog

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jenkins-x/jx-helpers/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/pkg/scmhelpers"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/pkg/errors"
)

// CreateBucketHTTPFn creates a function to transform a git URL to add the token and possible header function for accessing a git based bucket
func (t *TektonLogger) CreateBucketHTTPFn() func(string) (string, func(*http.Request), error) {
	return func(urlText string) (string, func(*http.Request), error) {
		headerFunc := func(*http.Request) {
			return
		}

		gitInfo, err := giturl.ParseGitURL(urlText)
		if err != nil {
			log.Logger().Warnf("Could not find the git token to access urlText %s due to: %s", urlText, err)
		}

		gitServerURL := gitInfo.HostURL()

		f := scmhelpers.Factory{
			GitServerURL: gitInfo.HostURL(),
			Owner:        gitInfo.Organisation,
			GitUsername:  t.GitUsername,
			GitToken:     t.GitToken,
		}

		err = f.FindGitToken()
		if err != nil {
			return "", headerFunc, errors.Wrapf(err, "failed to find git token for git URL %s", urlText)
		}
		gitKind := f.GitKind
		gitToken := f.GitToken
		tokenPrefix := ""
		if gitToken != "" {
			if gitKind == giturl.KindBitBucketServer {
				if f.GitUsername == "" {
					return "", headerFunc, errors.Wrapf(err, "no git username configured for git URL %s", urlText)
				}
				tokenPrefix = fmt.Sprintf("%s:%s", f.GitUsername, gitToken)
			} else if gitKind == giturl.KindGitlab {
				headerFunc = func(r *http.Request) {
					r.Header.Set("PRIVATE-TOKEN", gitToken)
				}
			} else if gitKind == giturl.KindGitHub && !gitInfo.IsGitHub() {
				// If we're on GitHub Enterprise, we need to put the token as a parameter to the URL.
				tokenPrefix = gitToken
			} else {
				tokenPrefix = gitToken
			}
		}
		if gitServerURL == "https://raw.githubusercontent.com" {
			if gitToken != "" {
				tokenPrefix = gitToken
			}
		}
		if tokenPrefix != "" {
			idx := strings.Index(urlText, "://")
			if idx > 0 {
				idx += 3
				urlText = urlText[0:idx] + tokenPrefix + "@" + urlText[idx:]
			}
		}
		return urlText, headerFunc, nil
	}
}
