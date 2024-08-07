package tektonlog

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

// CreateBucketHTTPFn creates a function to transform a git URL to add the token and possible header function for accessing a git based bucket
func (t *TektonLogger) CreateBucketHTTPFn() func(string) (string, func(*http.Request), error) {
	return func(urlText string) (string, func(*http.Request), error) {
		headerFunc := func(*http.Request) {
		}

		gitInfo, err := giturl.ParseGitURL(urlText)
		if err != nil {
			log.Logger().Warnf("Could not find the git token to access urlText %s due to: %s", urlText, err)
		}

		gitServerURL := gitInfo.HostURL()

		f := scmhelpers.Factory{
			GitServerURL: gitInfo.HostURL(),
			GitUsername:  t.GitUsername,
			GitToken:     t.GitToken,
		}

		err = f.FindGitToken()
		if err != nil {
			return "", headerFunc, fmt.Errorf("failed to find git token for git URL %s: %w", urlText, err)
		}
		gitKind := f.GitKind
		gitToken := f.GitToken
		tokenPrefix := ""
		if gitToken != "" {
			if gitKind == giturl.KindBitBucketServer {
				if f.GitUsername == "" {
					return "", headerFunc, fmt.Errorf("no git username configured for git URL %s: %w", urlText, err)
				}
				tokenPrefix = fmt.Sprintf("%s:%s", f.GitUsername, gitToken)
			} else if gitKind == giturl.KindGitlab {
				headerFunc = func(r *http.Request) {
					r.Header.Set("PRIVATE-TOKEN", gitToken)
				}
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
