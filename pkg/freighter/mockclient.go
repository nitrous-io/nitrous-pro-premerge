package freighter

// MockClient is a mock satisfying the freighter.Client interface.
type MockClient struct {
	Containers []*Container

	ListCalled int
	ListError  error

	CreateCalled     int
	CreateError      error
	ReceivedName     string
	ReceivedImage    string
	CreatedContainer *Container
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
