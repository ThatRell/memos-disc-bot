package main

type MemoPayload struct {
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Space       string       `json:"space,omitempty"`
}

type Attachment struct {
	Name     string `json:"name,omitempty"`
	Filename string `json:"filename"`
	Content  []byte `json:"content,omitempty"`
	Type     string `json:"type"`
}

type SpaceReq struct {
	Title string `json:"title"`
}

type Space struct {
	Name string `json:"name"`
}
