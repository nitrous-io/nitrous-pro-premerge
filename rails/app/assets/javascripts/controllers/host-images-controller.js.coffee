App.HostImagesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.images'

  # for push
  image: null
  repo: null
  closePushDialog: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      imageToPush: null
      pushRepo: null
      closePushDialog: false

  actions:
    pull: ->
      @transitionToRoute 'images.pull', queryParams: { preselectHost: @get('model.slug') }

    add: ->
      @transitionToRoute 'images.new', queryParams: { preselectHost: @get('model.slug') }

    update: ->
      selected = @get 'selected'
      host = @get 'model'

      fns = selected.map (record) ->
        (cb) ->
          App.Image.pull host, record.get('repo'), (resp) ->
            cb(null)
          , (jqxhr) -> # record.ajax
            App.postError(jqxhr.responseJSON, 'Failed to update image!')
            cb(jqxhr)

      async.series(fns)

    push: ->
      events = App.get 'eventsSocket'
      image = @get 'image'
      repo = @get 'repo'

      onImagePushFailed = (data) =>
        return unless data.image?.id == image.get('id')
        events.off 'image.push_failed', onImagePushFailed
        events.off 'image.pushed', onImagePushed
        errMsg = null
        if data.progress
          progress = JSON.parse(progress)
          errMsg = progress.errorDetail?.message
        else if data.error
          errMsg = data.error
        App.postError(errMsg, 'Failed to push image!')
        image.set 'pushing', false

      onImagePushed = (data) =>
        return unless data.image?.id == image.get('id')
        events.off 'image.push_failed', onImagePushFailed
        events.off 'image.pushed', onImagePushed
        name = image.get('displayName')
        if name == repo
          App.snackbar.post("Your image “#{name}” was successfully pushed.")
        else
          App.snackbar.post("Your image “#{name}” was successfully pushed to “#{repo}”.")
        image.set 'pushing', false
        @set 'closePushDialog', true

      events.on 'image.push_failed', onImagePushFailed
      events.on 'image.pushed', onImagePushed

      image.set 'pushing', true
      image.ajax('POST', 'push', repository: repo, async: 'true').fail (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to push!")
        image.set 'pushing', false

