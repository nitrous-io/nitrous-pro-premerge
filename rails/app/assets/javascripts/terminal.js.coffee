#= require_self
#= require lib/socket/socket-connection
#= require webfonts
#= require term

window.NitrousTerminal =
  term: null
  conn: null
  url: null
  firstConnect: true
  readonly: false

  resizeTimeout: null

  init: ->
    @conn = conn = new SocketConnection()
    conn.onbeforeopen   = => @willOpen()
    conn.onopen    = (ev) => @didOpen(ev)
    conn.onmessage = (ev) => @didReceiveMessage(ev)
    conn.onerror   = (ev) => @didFail(ev)
    conn.onclose   = (ev) => @didClose(ev)

    @term = term = new Terminal(80, 25)
    term.open(document.body)
    term.sizeToFit()
    term.write 'Connecting...'

    term.on 'resize', (cols, rows) ->
      return if conn.readyState != 1
      conn.send "r|#{cols} #{rows}"

    term.handler = (data) =>
      return if @readonly || conn.readyState != 1
      conn.send "d|#{data}"

    window.onresize = =>
      return unless @term
      if @resizeTimeout
        window.clearTimeout(@resizeTimeout)
      @resizeTimeout = window.setTimeout(=>
        @resizeTimeout = null
        @term.sizeToFit()
      , 50)

    window.onfocus = =>
      return unless @term
      Terminal.focus = @term
      @term.redrawCursor()

    window.onblur = =>
      return unless @term
      Terminal.focus = null
      @term.redrawCursor()

  open: (nobeforeopen) ->
    @conn.url = @url
    @conn.open(nobeforeopen)
    @conn.url = null

  retry: ->
    @conn.retry()

  destroy: ->
    @close()
    @term?.destroy()

  close: ->
    @term?.write("\r\n[Session closed]")
    @term?.removeAllListeners('resize')
    @term?.handler = ->
    @conn?.close()
    @willOpen = null

  focus: ->
    @term?.focus()

  willOpen: ->
    # override me

  didOpen: (ev) ->
    if @firstConnect
      @firstConnect = false
      @term.write '\x1b[r\x1b[H\x1b[J'
    else
      @term.write '\r\n'

  didReceiveMessage: (ev) ->
    type = ev.data.slice(0, 2)
    switch type
      when 'd|'
        @term.write ev.data.slice(2)
      when 'c|'
        @close()

  didFail: (ev) ->

  didClose: (ev) ->

window.webFontLoaded = ->
  NitrousTerminal.term?.sizeToFit()
