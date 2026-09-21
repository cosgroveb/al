package anylist

// List identifies an account-visible list.
type List struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Shared     bool   `json:"shared"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Quantity preserves the modern service representation in addition to display text.
type Quantity struct {
	Amount string `json:"amount"`
	Unit   string `json:"unit"`
	Raw    string `json:"raw"`
}

type Item struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Checked         bool      `json:"checked"`
	ModifiedAt      string    `json:"modifiedAt,omitempty"`
	Quantity        string    `json:"quantity"`
	QuantityDetails *Quantity `json:"quantityDetails,omitempty"`
	Notes           string    `json:"notes"`
	CategoryID      string    `json:"categoryId"`
	CategoryName    string    `json:"categoryName"`
}

// ListState is a snapshot used to resolve commands before writes.
type ListState struct {
	List
	Items           []Item     `json:"items"`
	Categories      []Category `json:"categories"`
	CategoryGroupID string     `json:"categoryGroupId"`
}

// ItemChange replaces only fields whose pointers are non-nil.
type ItemChange struct {
	ID         string
	Create     bool
	Remove     bool
	Name       *string
	Quantity   *string
	Notes      *string
	CategoryID *string
	Checked    *bool
}

type MutationResult struct {
	Index   *int   `json:"index,omitempty"`
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
	Item    *Item  `json:"item,omitempty"`
	List    *List  `json:"list,omitempty"`
}

type Candidate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Error contains only safe diagnostics; never retain upstream bodies or credentials.
type Error struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	HTTPStatus int         `json:"httpStatus,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
	Unknown    bool        `json:"-"`
}

func (e *Error) Error() string { return e.Message }
