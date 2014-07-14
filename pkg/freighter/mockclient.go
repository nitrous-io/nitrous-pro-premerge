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

	StartCalled int
	StartError  error

	StopCalled int
	StopError  error

	RestartCalled int
	RestartError  error

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

func (c *MockClient) Start(h *Host, id string) (*Container, error) {
	c.StartCalled = c.StartCalled + 1
	c.ReceivedID = id
	if c.StartError != nil {
		return nil, c.StartError
	}
	cntnr, _ := c.Info(h, id)
	if cntnr != nil {
		cntnr.State = StateRunning
	}
	return cntnr, nil
}

func (c *MockClient) Stop(h *Host, id string) (*Container, error) {
	c.StopCalled = c.StopCalled + 1
	c.ReceivedID = id
	if c.StopError != nil {
		return nil, c.StopError
	}
	cntnr, _ := c.Info(h, id)
	if cntnr != nil {
		cntnr.State = StateStopped
	}
	return cntnr, nil
}

func (c *MockClient) Restart(h *Host, id string) (*Container, error) {
	c.RestartCalled = c.RestartCalled + 1
	c.ReceivedID = id
	if c.RestartError != nil {
		return nil, c.RestartError
	}
	cntnr, _ := c.Info(h, id)
	if cntnr != nil {
		cntnr.State = StateRunning
	}
	return cntnr, nil
}
