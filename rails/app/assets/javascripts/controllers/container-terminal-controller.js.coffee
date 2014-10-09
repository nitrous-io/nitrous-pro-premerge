App.ContainerTerminalController = Em.ObjectController.extend
  queryParams: ['action', 'fullscreen']
  action: null
  fullscreen: false

  freighterdUrlBinding: Em.Binding.oneWay 'model.host.freighterdUrl'
  ready: Em.computed.notEmpty 'freighterdUrl'

  makeUrlAndConnect: (terminal) ->
    container = @get 'model'
    freighterdUrl = @get 'freighterdUrl'
    term = terminal.term

    action = @get('action') || 'shell'
    if ['shell', 'attach', 'logs'].indexOf(action) == -1
      action = 'shell'

    container.requestToken([action]).then (token) =>
      token = encodeURIComponent(token.token)

      protocol = location.protocol.replace('http', 'ws')
      url = "#{protocol}//#{freighterdUrl}/containers/#{container.get 'cid'}/#{action}?token=#{token}&width=#{term.cols}&height=#{term.rows}"

      switch action
        when 'shell'
          command = encodeURIComponent(getLoginCommand())
          url += "&command=#{command}"
        when 'logs'
          url += "&follow=true"
          terminal.readonly = true
          term.convertEol = true
        when 'attach'
          term.convertEol = true

      terminal.url = url
      # url obtained, resume connecting
      terminal.open(true)
    , (jqxhr) ->
      terminal.retry()
      App.postError(jqxhr.responseJSON, 'Could not connect to shell, retrying...')

getLoginCommand = (username) ->
  username ||= "action"
  "/usr/bin/script -q -c '/bin/login -f #{username} -p TERM=xterm-256color' /dev/null || /bin/login -f #{username} -p TERM=xterm-256color"
