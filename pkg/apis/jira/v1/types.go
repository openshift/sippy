package v1

const (
	LabelJiraAutomator = "ComponentAutomatedRegression"
)

type Issue struct {
	Expand    string    `json:"expand"`
	ID        string    `json:"id"`
	Self      string    `json:"self"`
	Key       string    `json:"key"`
	Fields    Fields    `json:"fields"`
	Changelog Changelog `json:"changelog"`
}

type Fields struct {
	IssueType      IssueType `json:"issuetype"`
	Project        Project   `json:"project"`
	Watches        Watches   `json:"watches"`
	Created        string    `json:"created"`
	ResolutionDate string    `json:"resolutiondate"`
	Priority       Priority  `json:"priority"`
	Labels         []string  `json:"labels"`
	Updated        string    `json:"updated"`
	Status         Status    `json:"status"`
	Description    string    `json:"description"`
	Summary        string    `json:"summary"`
	Creator        User      `json:"creator"`
	Reporter       User      `json:"reporter"`
}

type IssueType struct {
	Self        string `json:"self"`
	ID          string `json:"id"`
	Description string `json:"description"`
	IconURL     string `json:"iconUrl"`
	Name        string `json:"name"`
	Subtask     bool   `json:"subtask"`
	AvatarID    int    `json:"avatarId"`
}

type Project struct {
	Self            string          `json:"self"`
	ID              string          `json:"id"`
	Key             string          `json:"key"`
	Name            string          `json:"name"`
	ProjectTypeKey  string          `json:"projectTypeKey"`
	ProjectCategory ProjectCategory `json:"projectCategory"`
}

type ProjectCategory struct {
	Self        string `json:"self"`
	ID          string `json:"id"`
	Description string `json:"description"`
	Name        string `json:"name"`
}

type Watches struct {
	Self       string `json:"self"`
	WatchCount int    `json:"watchCount"`
	IsWatching bool   `json:"isWatching"`
}

type Priority struct {
	Self    string `json:"self"`
	IconURL string `json:"iconUrl"`
	Name    string `json:"name"`
	ID      string `json:"id"`
}

type Status struct {
	Self           string         `json:"self"`
	Description    string         `json:"description"`
	IconURL        string         `json:"iconUrl"`
	Name           string         `json:"name"`
	ID             string         `json:"id"`
	StatusCategory StatusCategory `json:"statusCategory"`
}

type StatusCategory struct {
	Self      string `json:"self"`
	ID        int    `json:"id"`
	Key       string `json:"key"`
	ColorName string `json:"colorName"`
	Name      string `json:"name"`
}

type User struct {
	Self        string `json:"self"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	Active      bool   `json:"active"`
	TimeZone    string `json:"timeZone"`
}

type Changelog struct {
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	Histories  []History `json:"histories"`
}

type History struct {
	ID      string `json:"id"`
	Author  User   `json:"author"`
	Created string `json:"created"`
	Items   []Item `json:"items"`
}

type Item struct {
	Field      string `json:"field"`
	FieldType  string `json:"fieldtype"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

type Component struct {
	Self                string `json:"self"`
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	Lead                User   `json:"lead"`
	AssigneeType        string `json:"assigneeType"`
	Assignee            User   `json:"assignee"`
	RealAssigneeType    string `json:"realAssigneeType"`
	RealAssignee        User   `json:"realAssignee"`
	IsAssigneeTypeValid bool   `json:"isAssigneeTypeValid"`
	Project             string `json:"project"`
	ProjectID           int    `json:"projectId"`
	Archived            bool   `json:"archived"`
	Deleted             bool   `json:"deleted"`
}
