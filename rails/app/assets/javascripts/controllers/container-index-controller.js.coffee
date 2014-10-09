App.ContainerIndexController = Em.ObjectController.extend App.PerformActionMixin,
  snapshotImageName: null
  closeSnapshotDialog: false
  snapshotting: false

  clear: ->
    setProperties:
      snapshotImageName: null
      closeSnapshotDialog: false
      snapshotting: false

  actions:
    start: ->
      @performAction('POST', 'start', 'starting')

    restart: ->
      @performAction('POST', 'restart', 'restarting')

    stop: ->
      @performAction('POST', 'stop', 'stopping')

    snapshot: ->
      name = @get 'snapshotImageName'
      record = @get 'model'
      @set 'snapshotting', true
      record.ajax('POST', 'commit', name: name).then(=>
        App.snackbar.post("Your snapshot “#{name}” was successfully created.")
        @set 'closeSnapshotDialog', true
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to snapshot!")
      ).finally =>
        @set 'snapshotting', false

    terminate: ->
      name = @get 'model.displayName'
      @performAction('DELETE', null, 'terminating').then ->
        App.snackbar.post("Your container “#{name}” was successfully terminated.")

    openLogs: ->
      @transitionToRoute 'container.terminal', queryParams: { action: 'logs' }

    openAttach: ->
      @transitionToRoute 'container.terminal', queryParams: { action: 'attach' }

    openTerminal: ->
      @transitionToRoute 'container.terminal', queryParams: { action: null }
