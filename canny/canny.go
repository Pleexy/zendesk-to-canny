package canny

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

//CreatePost contains fields/params for a posts/create api call
type CreatePost struct {
	AuthorID  string   `json:"authorID"`
	BoardID   string   `json:"boardID"`
	Details   string   `json:"details"`
	Title     string   `json:"title"`
	ImageURLs []string `json:"imageURLs,omitempty"`
}

type createPostRequest struct {
	APIKey string `json:"apiKey"`
	CreatePost
}

//CreateComment contains fields/params for a comments/create api call
type CreateComment struct {
	AuthorID  string   `json:"authorID"`
	PostID    string   `json:"postID"`
	Value     string   `json:"value"`
	ImageURLs []string `json:"imageURLs,omitempty"`
	ParentID  string   `json:"parentID,omitempty"`
}

type createCommentRequest struct {
	APIKey string `json:"apiKey"`
	CreateComment
}

//CreateVote contains fields/params for a votes/create api call
type CreateVote struct {
	PostID  string `json:"postID"`
	VoterID string `json:"voterID"`
}

type createVoteRequest struct {
	APIKey string `json:"apiKey"`
	CreateVote
}

//FindOrCreateUser contains fields/params for find_or_create api call
type FindOrCreateUser struct {
	AvatarURL string    `json:"avatarURL,omitempty"`
	Created   time.Time `json:"created,omitempty"`
	Email     string    `json:"email,omitempty"`
	Name      string    `json:"name,omitempty"`
	UserID    string    `json:"userID,omitempty"`
}

type findOrCreateUserRequest struct {
	APIKey string `json:"apiKey"`
	FindOrCreateUser
}
type response struct {
	ID string
}

//Client provides methods to interact with canny.io api
type Client struct {
	APIKey  string
	BaseURL string
}

// CreatePost create a new post in Canny and returns its id or error
func (s *Client) CreatePost(post CreatePost) (string, error) {
	req := &createPostRequest{
		APIKey:     s.APIKey,
		CreatePost: post,
	}
	var resp response
	err := s.post(fmt.Sprintf("%s/api/v1/posts/create", s.BaseURL), req, &resp)
	if err != nil {
		return "", err
	}
	return resp.ID, err
}

// CreateComment create a new comment in Canny and returns its id or error
func (s *Client) CreateComment(comment CreateComment) (string, error) {
	req := &createCommentRequest{
		APIKey:        s.APIKey,
		CreateComment: comment,
	}
	var resp response
	err := s.post(fmt.Sprintf("%s/api/v1/comments/create", s.BaseURL), req, &resp)
	if err != nil {
		return "", err
	}
	return resp.ID, err
}

// CreateVote create a new vote in Canny and returns its id or error
func (s *Client) CreateVote(vote CreateVote) error {
	req := &createVoteRequest{
		APIKey:     s.APIKey,
		CreateVote: vote,
	}
	var resp string
	err := s.post(fmt.Sprintf("%s/api/v1/votes/create", s.BaseURL), req, &resp)
	if err != nil {
		return err
	}
	if resp != "success" {
		return fmt.Errorf("unknown error while creating vote for postID=%s voterID=%s", vote.PostID, vote.VoterID)
	}
	return nil
}

// FindOrCreateUser finds or creates a user
func (s *Client) FindOrCreateUser(user FindOrCreateUser) (string, error) {
	req := &findOrCreateUserRequest{
		APIKey:           s.APIKey,
		FindOrCreateUser: user,
	}
	var resp response
	err := s.post(fmt.Sprintf("%s/api/v1/users/find_or_create", s.BaseURL), req, &resp)
	if err != nil {
		return "", err
	}
	return resp.ID, err
}

func (s *Client) post(url string, src interface{}, dst interface{}) error {
	body, err := json.Marshal(src)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot read response body:%w, request:%s", err, body)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("error while making request to %s: %d - %s, request:%s", url, resp.StatusCode, resBody, body)
	}
	if strDest, ok := dst.(*string); ok {
		*strDest = string(resBody)
	} else {
		err = json.Unmarshal(resBody, dst)
		if err != nil {
			return fmt.Errorf("cannot unmarshal response to json:%w, response:%s, request:%s", err, resBody, body)
		}
	}
	return nil
}
