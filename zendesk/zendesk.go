package zendesk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"
)

//Client implements client to access Zendesk API
type Client struct {
	Username string
	Password string
	BaseURL  string
	Users    map[int64]*User
}

//User describes fields of Zendesk User that are used by migration
type User struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	ExternalID string    `json:"external_id"`
}

//Post describes fields of Zendesk Post that are used by migration
type Post struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Details      string `json:"details"`
	AuthorID     int64  `json:"author_id"`
	VoteCount    int    `json:"vote_count"`
	CommentCount int    `json:"comment_count"`
	Comments     []*Comment
	UserVotes    []*Vote
	Author       *User
}

//Comment describes fields of Zendesk Comment that are used by migration
type Comment struct {
	ID       int64
	Body     string
	AuthorID int64 `json:"author_id"`
	Author   *User
}

//Vote describes fields of Zendesk Vote that are used by migration
type Vote struct {
	ID     int64 `json:"id"`
	UserID int64 `json:"user_id"`
	User   *User
}

//ResponseFooter describe fields returned with every list request
type ResponseFooter struct {
	NextPage string `json:"next_page"`
	Count    int    `json:"count"`
}

type usersResponse struct {
	Users []*User
	ResponseFooter
}

type postsResponse struct {
	Posts []Post
	ResponseFooter
}
type commentsResponse struct {
	Comments []*Comment
	ResponseFooter
}
type votesResponse struct {
	Votes []*Vote
	ResponseFooter
}

//PostLoadedCallback describe a function that is called for every loaded post.
type PostLoadedCallback func(post *Post)

//PostLoadingErrorCallback describe a function that is called for error occured during post loading
type PostLoadingErrorCallback func(err error)

//GetPosts return all posts for specific topic
func (s *Client) GetPosts(topic string, parallel int, postCB PostLoadedCallback, errCB PostLoadingErrorCallback) ([]*Post, []error, error) {
	posts := make([]*Post, 0)
	errs := make([]error, 0)
	if s.Users == nil {
		s.Users = make(map[int64]*User)
	}
	loadPostsCh := make(chan *Post)
	loadedPostsCh, usersCh, errorsCh := s.detailsLoader(loadPostsCh, parallel)
	var routinesWG sync.WaitGroup
	routinesWG.Add(3)
	go func() {
		for loadedPost := range loadedPostsCh {
			posts = append(posts, loadedPost)
			if postCB != nil {
				postCB(loadedPost)
			}
		}
		routinesWG.Done()
	}()
	go func() {
		for loadErr := range errorsCh {
			errs = append(errs, loadErr)
			if errCB != nil {
				errCB(loadErr)
			}
		}
		routinesWG.Done()
	}()
	usersToLoad := make([]int64, 0)
	go func() {
		for userID := range usersCh {
			if _, ok := s.Users[userID]; !ok {
				s.Users[userID] = nil // we are loading it
				usersToLoad = append(usersToLoad, userID)
			}
		}
		routinesWG.Done()
	}()
	url := fmt.Sprintf("%s/api/v2/community/topics/%s/posts.json?sort_by=created_at", s.BaseURL, topic)
	page := 0
	var fatalError error
	for true {
		var response postsResponse
		err := s.get(url, &response)
		if err != nil {
			fatalError = fmt.Errorf("error while getting page %d of posts: %w", page, err)
			break
		}
		if response.Posts == nil && response.NextPage != "" {
			fatalError = fmt.Errorf("posts are not found on page %d while next page is present", page)
			break
		}
		for _, post := range response.Posts {
			postVar := post
			loadPostsCh <- &postVar
		}
		if response.NextPage == "" {
			break
		}
		url = response.NextPage
		page++
	}
	close(loadPostsCh)
	routinesWG.Wait()
	err := s.loadUsers(usersToLoad)
	if err != nil {
		return nil, nil, err
	}
	s.setUsers(posts)
	return posts, errs, fatalError
}

func (s *Client) setUsers(posts []*Post) {
	for _, post := range posts {
		post.Author = s.Users[post.AuthorID]
		for _, comment := range post.Comments {
			comment.Author = s.Users[comment.AuthorID]
		}
		for _, vote := range post.UserVotes {
			vote.User = s.Users[vote.UserID]
		}
	}
}

func (s *Client) detailsLoader(postsCh <-chan *Post, inParallel int) (<-chan *Post, <-chan int64, <-chan error) {
	errCh := make(chan error)
	resCh := make(chan *Post)
	usersCh := make(chan int64)
	var routinesWG sync.WaitGroup
	routinesWG.Add(inParallel)
	go func() {
		routinesWG.Wait()
		close(errCh)
		close(usersCh)
		close(resCh)
	}()
	for i := 0; i < inParallel; i++ {
		go func() {
			for post := range postsCh {
				usersCh <- post.AuthorID
				if post.CommentCount > 0 {
					comments, err := s.getComments(post.ID)
					if err != nil {
						errCh <- err
						continue
					}
					post.Comments = comments
					for _, comment := range comments {
						usersCh <- comment.AuthorID
					}
				}
				if post.VoteCount > 0 {
					votes, err := s.getVotes(post.ID)
					if err != nil {
						errCh <- err
						continue
					}
					post.UserVotes = votes
					for _, v := range votes {
						usersCh <- v.UserID
					}
				}
				resCh <- post
			}
			routinesWG.Done()
		}()
	}
	return resCh, usersCh, errCh
}

func (s *Client) loadUsers(ids []int64) error {
	//split ids into batches
	batchSize := 100
	batches := make([][]int64, 0, (len(ids)+batchSize-1)/batchSize)

	for batchSize < len(ids) {
		ids, batches = ids[batchSize:], append(batches, ids[0:batchSize])
	}
	batches = append(batches, ids)
	for _, batch := range batches {
		url := fmt.Sprintf("%s/api/v2/users/show_many.json?ids=%s", s.BaseURL, idsToString(batch, ","))
		var response usersResponse
		err := s.get(url, &response)
		if err != nil {
			return fmt.Errorf("error while getting batch of users: %w", err)
		}
		for _, user := range response.Users {
			s.Users[user.ID] = user
		}
	}
	return nil
}

//getComments return all comments for specific post
func (s *Client) getComments(postID int64) ([]*Comment, error) {
	comments := make([]*Comment, 0)
	url := fmt.Sprintf("%s/api/v2/community/posts/%d/comments.json?sort_by=created_at", s.BaseURL, postID)
	page := 0
	for true {
		var response commentsResponse
		err := s.get(url, &response)
		if err != nil {
			return nil, fmt.Errorf("error while getting page %d of comments for postID=%d: %w", page, err, postID)
		}
		if response.Comments == nil && response.NextPage != "" {
			return nil, fmt.Errorf("Comments are not found on page %d while next page is present, postID=%d", page, postID)
		}
		comments = append(comments, response.Comments...)
		if response.NextPage == "" {
			return comments, nil
		}
		url = response.NextPage
		page++
	}
	return nil, nil
}

//getVotes return all votes, as a list of users voted for specific post
func (s *Client) getVotes(postID int64) ([]*Vote, error) {
	votes := make([]*Vote, 0)
	url := fmt.Sprintf("%s/api/v2/community/posts/%d/votes.json?sort_by=created_at", s.BaseURL, postID)
	page := 0
	for true {
		var response votesResponse
		err := s.get(url, &response)
		if err != nil {
			return nil, fmt.Errorf("error while getting page %d of votes for postID=%d: %w", page, err, postID)
		}
		if response.Votes == nil && response.NextPage != "" {
			return nil, fmt.Errorf("Posts are not found on page %d while next page is present, postID=%d", page, postID)
		}
		votes = append(votes, response.Votes...)
		if response.NextPage == "" {
			return votes, nil
		}
		url = response.NextPage
		page++
	}
	return nil, nil
}

func (s *Client) get(url string, dst interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.Username, s.Password)
	cli := &http.Client{}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot read response body:%w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("error while making request to %s: %d - %s", url, resp.StatusCode, body)
	}
	err = json.Unmarshal(body, dst)
	if err != nil {
		return fmt.Errorf("cannot unmarshal response to json:%w, response:%s", err, body)
	}
	return nil
}

func idsToString(ids []int64, delim string) string {
	var buffer bytes.Buffer
	for i := 0; i < len(ids); i++ {
		buffer.WriteString(strconv.FormatInt(ids[i], 10))
		if i != len(ids)-1 {
			buffer.WriteString(delim)
		}
	}

	return buffer.String()
}
