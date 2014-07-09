package freighter

// MockClient is a mock satisfying the freighter.Client interface.
type MockClient struct {
	Containers []*Container

	ListCalled int
	ListError  error
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
