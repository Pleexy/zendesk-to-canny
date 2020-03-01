# Zendesk to Canny migration

__Migrate Zendesk HC Community posts to Canny.io
* Migrates all the posts for specified topics
* Migrates all the comments and votes
* Finds or creates corresponding users in canny.io
* Saves all processed entities (posts, comments, votes) in a state file and skip them on the next run. It won't create duplicate records if state file is available.

## Instalation
```bash
$ go get github.com/Pleexy/zendesk-to-canny
```
## Usage
```bash
$ zendesk-to-canny -z-url zendesk_url -z-username zendesk_username -z-password zendesk_userpassword \
                   -c-key canny_api_key \
                   zendesk_topic_id:canny_board_id [zendesk_topic_id:canny_board_id [zendesk_topic_id:canny_board_id...]]
  Options:
    --z-url url      	      Required. Zendesk URL (e.g. https://your_company.zendesk.com) 
    --z-username username     Required. User name to access Zendesk API
    --z-password pass         Required. User password to access Zendesk API
    --c-key apiKey            Required. Canny API key
    --c-url url               Optional. Canny APU URL. Default https://canny.io
    --default-user userID     Optional. Default user id (from Canny) which will be used for posts and comments where user is missing in Zendesk.
                                 If not provided, posts and comments without user will be skipped.
    --parallel n              Optional. Number of parallel loads from Zendesk. Default is 10
    --state file              Optional. State file. Default ./state.json
    --agent zendeskID:cannyID Optional. Specify mapping between Zendesk agents and Canny admins, if post/comments/votes authored by admins.
                                Can be provided multiple times.
    --verbose                 Print verbose logging
    --help                    Print usage
  Arguments:
    Pairs of zendesk_topic_id:canny_board_id, where
      zendesk_topic_id - Zendesk Help Center topic ID to load posts from (e.g. 115000153468-Integrations)
      canny_board_id   - ID of Canny board to create posts at. Multiple Zendesk topics can be mapped to the same Canny board.
```
