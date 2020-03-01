package main

import (
	"fmt"
	"github.com/Pleexy/zendesk-to-canny/canny"
	"github.com/Pleexy/zendesk-to-canny/zendesk"
	flag "github.com/spf13/pflag"
	"log"
	"os"
	"strconv"
	"strings"
)

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr,
			`Usage: zendesk-to-canny -z-url zendesk_url -z-username zendesk_username -z-password zendesk_userpassword -c-key canny_api_key \
                              zendesk_topic_id:canny_board_id [zendesk_topic_id:canny_board_id [zendesk_topic_id:canny_board_id...]]
Options:
  --z-url url      	       Required. Zendesk URL (e.g. https://your_company.zendesk.com) 
  --z-username username     Required. User name to access Zendesk API
  --z-password pass         Required. User password to access Zendesk API
  --c-key apiKey       	   Required. Canny API key
  --c-url url     		   Optional. Canny APU URL. Default https://canny.io
  --default-user userID     Optional. Default user id (from Canny) which will be used for posts and comments where user is missing in Zendesk.
                           If not provided, posts and comments without user will be skipped.
  --parallel n              Optional. Number of parallel loads from Zendesk. Default is 10
  --state file   		   Optional. State file. Default ./state.json
  --agent zendeskID:cannyID Optional. Specify mapping between Zendesk agents and Canny admins, if post/comments/votes authored by admins.
                           Can be provided multiple times.
  --verbose         Print verbose logging
  --help            Print usage
Arguments:
  Pairs of zendesk_topic_id:canny_board_id, where
	zendesk_topic_id - Zendesk Help Center topic ID to load posts from (e.g. 115000153468-Integrations)
    canny_board_id - ID of Canny board to create posts at. Multiple Zendesk topics can be mapped to the same Canny board.
`)
	}

	helpPtr := flag.Bool("help", false, "")
	verbosePtr := flag.Bool("verbose", false, "")
	zURLPrt := flag.String("z-url", "", "")
	zUsernamePtr := flag.String("z-username", "", "")
	zPasswordPtr := flag.String("z-password", "", "")
	cKeyPtr := flag.String("c-key", "", "")
	cURLPtr := flag.String("c-url", "https://canny.io", "")
	statePtr := flag.String("state", "./state.json", "")
	defaultUserPtr := flag.String("default-user", "", "")
	parallelPtr := flag.Int("parallel", 10, "")
	agentsPtr := flag.StringSlice("agent", []string{}, "")

	flag.Parse()

	if *helpPtr {
		flag.Usage()
		os.Exit(0)
	}

	if *zURLPrt == "" || *zUsernamePtr == "" || *zPasswordPtr == "" || *cKeyPtr == "" {
		fmt.Fprint(os.Stderr, "-z-url, -z-username, -z-password, -c-key are required")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		_, _ = fmt.Fprint(os.Stderr, "at least one pair of zendesk_topic_id:canny_board_id MUST be provided")
		flag.Usage()
		os.Exit(1)
	}
	topics := make(map[string]string)
	for _, arg := range args {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			_, _ = fmt.Fprint(os.Stderr, "invalid arguments format")
			flag.Usage()
			os.Exit(1)
		}
		topics[parts[0]] = parts[1]
	}
	var agents map[int64]string
	if len(*agentsPtr) > 0 {
		agents = make(map[int64]string)
		for _, agent := range *agentsPtr {
			parts := strings.Split(agent, ":")
			if len(parts) != 2 {
				_, _ = fmt.Fprintf(os.Stderr, "invalid agent format %s", agent)
				flag.Usage()
				os.Exit(1)
			}
			zID, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "invalid agent format %s", agent)
				flag.Usage()
				os.Exit(1)
			}
			agents[zID] = parts[1]
		}
	}

	cClient := &canny.Client{
		APIKey:  *cKeyPtr,
		BaseURL: *cURLPtr,
	}
	zClient := &zendesk.Client{
		Username: *zUsernamePtr,
		Password: *zPasswordPtr,
		BaseURL:  *zURLPrt,
	}
	migration := &Migration{
		ZClient:       zClient,
		CClient:       cClient,
		Topics:        topics,
		Verbose:       *verbosePtr,
		DefaultUserID: *defaultUserPtr,
		ParallelLoad:  *parallelPtr,
		StateFile:     *statePtr,
		Logger:        log.New(os.Stdout, "", 0),
		UserMapping:   agents,
	}

	err := migration.Migrate()
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
	}
}
