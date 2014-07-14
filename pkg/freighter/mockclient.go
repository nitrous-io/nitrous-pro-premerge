package freighter

// MockClient is a mock satisfying the freighter.Client interface.
type MockClient struct {
	Containers []*Container

	ListCalled int
	ListError  error

	CreateCalled     int
	CreateError      error
	CreatedContainer *Container

	InfoCalled int
	InfoError  error

	ReceivedID    string
	ReceivedName  string
	ReceivedImage string
}

func (c *MockClient) List(h *Host) ([]*Container, error) {
	c.ListCalled = c.ListCalled + 1
	if c.ListError != nil {
		return nil, c.ListError
	}
	if c.Containers == nil {
		return []*Container{}, nil
	}
	return c.Containers, nil
}

func (c *MockClient) Create(h *Host, name, image string) (*Container, error) {
	c.CreateCalled = c.CreateCalled + 1
	c.ReceivedName = name
	c.ReceivedImage = image
	return c.CreatedContainer, c.CreateError
}

func (c *MockClient) Info(h *Host, id string) (*Container, error) {
	c.InfoCalled = c.InfoCalled + 1
	c.ReceivedID = id
	if c.InfoError != nil {
		return nil, c.InfoError
	}
	for _, c := range c.Containers {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, ErrNotFound
}
