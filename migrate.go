package main

import (
	"encoding/json"
	"fmt"
	"github.com/Pleexy/zendesk-to-canny/canny"
	"github.com/Pleexy/zendesk-to-canny/zendesk"
	"github.com/kennygrant/sanitize"
	"io/ioutil"
	"log"
	"os"
)

// Migration contains migration parameters and methods
type Migration struct {
	ZClient       *zendesk.Client
	CClient       *canny.Client
	Topics        map[string]string
	Verbose       bool
	DefaultUserID string
	ParallelLoad  int
	StateFile     string
	UserMapping   map[int64]string
	state         map[string]map[string]string // contains mapping of [zendesk_topic: ['<zendesk_type><zendesk_id>':'canny_id']]
	Logger        *log.Logger
}

//Migrate performs a migration for specified topics
func (s *Migration) Migrate() error {
	if err := s.loadState(); err != nil {
		return fmt.Errorf("cannot load State file:%w", err)
	}
	if s.UserMapping == nil {
		s.UserMapping = make(map[int64]string)
	}
	for zTopic, cBoard := range s.Topics {
		var success, fail int
		s.Logger.Printf("Migrating topic '%s' to board '%s'", zTopic, cBoard)
		posts, errs, fatalError := s.ZClient.GetPosts(zTopic, s.ParallelLoad, s.printZPost, s.printErr)
		if fatalError != nil {
			s.Logger.Printf("FATAL ERROR while loading posts for %s, skipping - %v", zTopic, fatalError)
			continue
		}
		s.Logger.Printf("Loaded %d posts with %d error", len(posts), len(errs))
		for _, post := range posts {
			err := s.migratePost(post, zTopic, cBoard)
			if err != nil {
				s.Logger.Printf("\tError while creating Canny post '%s' from Zendesk post %d: %v", post.Title, post.ID, err)
				fail++
			} else {
				if s.Verbose {
					s.Logger.Printf("\tMigrated post '%s'", post.Title)
				}
				success++
			}
		}
		s.Logger.Printf("Migrated topic '%s' to board '%s': %d posts, %d errors", zTopic, cBoard, success, fail)
	}
	if err := s.saveState(); err != nil {
		s.Logger.Print(s.state)
		return fmt.Errorf("cannot save State file:%w. State is printed above, add to state file manually before repeating operation", err)
	}
	return nil
}

func (s *Migration) loadState() error {
	if s.StateFile == "" || !fileExists(s.StateFile) {
		s.state = make(map[string]map[string]string)
		return nil
	}
	raw, err := ioutil.ReadFile(s.StateFile)
	if err != nil {
		return err
	}
	err = json.Unmarshal(raw, &s.state)
	return err
}

func (s *Migration) saveState() error {
	data, err := json.MarshalIndent(s.state, "", " ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(s.StateFile, data, 0644)
}

func (s *Migration) migratePost(post *zendesk.Post, zTopic, cBoard string) error {
	var err error
	postID := s.getIDFromState(zTopic, "post", post.ID)
	if postID == "" {
		postID, err = s.createPost(post, cBoard)
		if err != nil {
			return err
		}
		s.saveIDToState(zTopic, "post", post.ID, postID)
	} else {
		if s.Verbose {
			s.Logger.Printf("\tpost '%s' is found in State - skipping", post.Title)
		}
	}
	for _, comment := range post.Comments {
		commentID := s.getIDFromState(zTopic, "comment", comment.ID)
		if commentID != "" {
			if s.Verbose {
				s.Logger.Printf("\tComment '%d' is found in State - skipping", comment.ID)
			}
			continue
		}
		commentID, err := s.createComment(comment, postID)
		if err != nil {
			return err
		}
		s.saveIDToState(zTopic, "comment", comment.ID, commentID)
	}

	for _, vote := range post.UserVotes {
		voteSuccess := s.getIDFromState(zTopic, "vote", vote.ID)
		if voteSuccess != "" || vote.User == nil {
			continue
		}
		voteSuccess, err := s.createVote(vote, postID)
		if err != nil {
			return err
		}
		s.saveIDToState(zTopic, "vote", vote.ID, voteSuccess)
	}
	return nil
}

func (s *Migration) createPost(post *zendesk.Post, cBoard string) (string, error) {
	userID, err := s.resolveUser(post.Author, "post")
	if err != nil {
		return "", err
	}
	return s.CClient.CreatePost(canny.CreatePost{
		AuthorID: userID,
		BoardID:  cBoard,
		Details:  sanitizeString(post.Details),
		Title:    sanitizeString(post.Title),
	})
}

func (s *Migration) createComment(comment *zendesk.Comment, postID string) (string, error) {
	userID, err := s.resolveUser(comment.Author, "comment")
	if err != nil {
		return "", err
	}
	return s.CClient.CreateComment(canny.CreateComment{
		AuthorID: userID,
		PostID:   postID,
		Value:    sanitizeString(comment.Body),
	})
}
func (s *Migration) createVote(vote *zendesk.Vote, postID string) (string, error) {
	userID, err := s.resolveUser(vote.User, "vote")
	if err != nil {
		return "", err
	}
	err = s.CClient.CreateVote(canny.CreateVote{
		PostID:  postID,
		VoterID: userID,
	})
	if err != nil {
		return "", err
	}
	return "s", nil
}

func (s *Migration) resolveUser(user *zendesk.User, objType string) (string, error) {
	var userID string
	var err error
	if user == nil {
		if s.DefaultUserID == "" {
			return "", fmt.Errorf("%s doesn't have a user and default user is not specified", objType)
		}
		userID = s.DefaultUserID
	} else {
		userID, err = s.findOrCreateUser(user)
		if err != nil {
			return "", err
		}
	}
	return userID, nil
}

func (s *Migration) findOrCreateUser(user *zendesk.User) (string, error) {
	if knownUserID := s.UserMapping[user.ID]; knownUserID != "" {
		return knownUserID, nil
	}
	userID, err := s.CClient.FindOrCreateUser(canny.FindOrCreateUser{
		Created: user.CreatedAt,
		Email:   user.Email,
		Name:    user.Name,
		UserID:  user.ExternalID,
	})
	if err != nil {
		return "", err
	}
	s.UserMapping[user.ID] = userID
	return userID, nil
}

func (s *Migration) getIDFromState(zTopic string, objType string, id int64) string {
	if s.state[zTopic] == nil {
		return ""
	}
	return s.state[zTopic][formatKey(objType, id)]
}

func (s *Migration) saveIDToState(zTopic string, objType string, id int64, cannyID string) {
	if s.state[zTopic] == nil {
		s.state[zTopic] = make(map[string]string)
	}
	s.state[zTopic][formatKey(objType, id)] = cannyID
}

func formatKey(objType string, id int64) string {
	return fmt.Sprintf("%s_%d", objType, id)
}

func (s *Migration) printZPost(post *zendesk.Post) {
	if s.Verbose {
		s.Logger.Printf("\tLoaded post %d: '%s' with %d comments and %d votes", post.ID, post.Title, len(post.Comments), len(post.UserVotes))
	}
}
func (s *Migration) printErr(err error) {
	s.Logger.Printf("\tError:%v", err)
}

func sanitizeString(htmlStr string) string {
	return sanitize.HTML(htmlStr)
}

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
