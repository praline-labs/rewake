package inbox

// Availability identifies a service-generated notification, not an agent reply.
// Its identity is frozen at announcement; CLI state is fetched by FromEpoch.
type Availability struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Harness  string `json:"harness"`
	Room     string `json:"room"`
	CWD      string `json:"cwd"`
	Existing bool   `json:"alreadyAvailable"`
}
