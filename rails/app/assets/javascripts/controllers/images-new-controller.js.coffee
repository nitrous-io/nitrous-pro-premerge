App.ImagesNewController = Em.Controller.extend
  queryParams: ['preselectHost']
  preselectHost: null

  selectedHostSlug: null
  name: null
  slug: null
  image: null
  remoteURL: null
  saving: false

  clear: ->
    @setProperties
      selectedHostSlug: null
      name: null
      slug: null
      image: null
      saving: false

  actions:
    cancel: ->
      if slug = @get('preselectHost')
        @transitionToRoute 'host.images', slug
      else
        @transitionToRoute 'home.index'

    create: (file) ->
      @set 'saving', true
      name = @get 'name'
      slug = @get 'slug'
      hostSlug = @get 'selectedHostSlug'

      data = new FormData()
      data.append 'name', name
      data.append 'slug', slug
      data.append 'async', true
      data.append 'file', file

      # send a remoteURL only when a file is not set
      if !file && remoteURL = @get 'remoteURL'
        data.append 'remote', remoteURL

      events = App.get 'eventsSocket'

      image = App.Image.create(name: name, slug: slug)
      @set 'image', image

      onImageBuildFailed = (data) =>
        imageData = data.image
        return unless imageData.id == image.get('id')
        events.off 'image.build_failed', onImageBuildFailed
        events.off 'image.built', onImageBuilt
        @set 'image', null
        errMsg = null
        if data.progress
          progress = JSON.parse(progress)
          errMsg = progress.errorDetail?.message
        App.postError(errMsg, 'Failed to build image!')
        @set 'saving', false

      onImageBuilt = (data) =>
        imageData = data.image
        return unless imageData.id == image.get('id')
        events.off 'image.build_failed', onImageBuildFailed
        events.off 'image.built', onImageBuilt
        @transitionToRoute 'host.images', hostSlug
        App.snackbar.post("Your image “#{name}” was built successfully.")
        @set 'saving', false

      events.on 'image.build_failed', onImageBuildFailed
      events.on 'image.built', onImageBuilt

      Em.RSVP.Promise.cast(Em.$.ajax(
        type: 'POST'
        url: "/api/v0/hosts/#{hostSlug}/images"
        data: data
        cache: false
        contentType: false
        processData: false
        headers: App.Image.adapter.get('headers')
      )).then (json) =>
        image.load(json.image)
      , (jqxhr) -> # Em.RSVP.Promise.cast
        App.postError(jqxhr.responseJSON, 'Failed to build image!')
        @set 'saving', false
