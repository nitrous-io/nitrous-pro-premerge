App.ImagesPullController = Em.Controller.extend
  queryParams: ['preselectHost']
  preselectHost: null

  selectedHostSlug: null
  repo: null
  image: null
  saving: false

  clear: ->
    @setProperties
      selectedHostSlug: null
      repo: null
      image: null
      saving: false

  selectedHost: (->
    if slug = @get 'selectedHostSlug'
      App.get('hosts').findBy 'slug', slug
  ).property 'selectedHostSlug', 'App.hosts.@each'

  actions:
    cancel: ->
      if slug = @get('preselectHost')
        @transitionToRoute 'host.images', slug
      else
        @transitionToRoute 'home.index'

    create: ->
      @set 'saving', true
      events = App.get 'eventsSocket'

      repo = @get 'repo'
      unless repo.match /^.*:.+$/
        repo = repo.replace /:?$/, ':latest'
      @set 'repo', repo

      App.Image.pull(@get('selectedHost'), repo, false).then (image) =>
        @set 'image', image

        onImagePullFailed = (data) =>
          imageData = data.image
          return unless imageData.id == image.get('id')
          events.off 'image.pull_failed', onImagePullFailed
          events.off 'image.pulled', onImagePulled
          @set 'image', null
          errMsg = null
          if data.progress
            progress = JSON.parse(progress)
            errMsg = progress.errorDetail?.message
          App.postError(errMsg, 'Failed to pull image!')
          @set 'saving', false

        onImagePulled = (data) =>
          imageData = data.image
          return unless imageData.id == image.get('id')
          events.off 'image.pull_failed', onImagePullFailed
          events.off 'image.pulled', onImagePulled
          @transitionToRoute 'host.images', @get('selectedHostSlug')
          @set 'saving', false

        events.on 'image.pull_failed', onImagePullFailed
        events.on 'image.pulled', onImagePulled
