App.ContainersNewView = Em.View.extend
  elementId: 'containers-new-view'
  classNames: ['ui', 'card']

  selectedOption: null
  selectedHostImage: null
  selectedBaseImage: null
  customImage: null

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  volumeMountpointInputPattern: '[^\\s\\?\\\\*%:\\|"\'<>$~]+'
  volumeMountpointInputTitle: 'Whitespace and the following characters are not allowed: \\ ? * % : | \" \' < > $ ~'

  image: (->
    option = @get 'selectedOption'
    switch option
      when 'host-image'
        unless @get 'controller.hostImagesIsEmpty'
          @get 'selectedHostImage'
      when 'base-image'
        @get 'selectedBaseImage'
      else
        @get 'customImage'
  ).property 'selectedOption', 'selectedHostImage', 'selectedBaseImage', 'customImage', 'controller.hostImagesIsEmpty'

  baseImageOptionDisabled: (->
    !App.get('baseImages') || @get('controller.saving')
  ).property 'App.baseImages', 'controller.saving'
  hostImageOptionDisabled: Em.computed.or('controller.hostImagesIsEmpty', 'controller.saving')

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedHostSlug') && @get('image'))
  ).property 'controller.name', 'controller.slug', 'controller.selectedHostSlug', 'image'

  addVolumeMountButtonDisabled: Em.computed.empty('controller.selectedVolume')
  addPortBindingButtonDisabled: Em.computed.empty('controller.containerPort')

  addEnvVarButtonDisabled: (->
    !(@get('controller.envKey') && @get('controller.envValue'))
  ).property 'controller.envKey', 'controller.envValue'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.image.progress.progressDetail.start')

  hostsDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      slug = @get('controller.preselectHost')
      if slug && !@get('selectedHostSlug')
        @set 'controller.selectedHostSlug', slug
      @$('select[name=host]')?.change()
      @$('select[name=containers-new-option-host-image]')?.change()
      @$('select[name=containers-new-selected-volume]')?.change()
  ).observes 'App.hosts.@each'

  preselectHostDidChange: (->
    if slug = @get('controller.preselectHost')
      @set 'controller.selectedHostSlug', slug
  ).observes 'controller.preselectHost'

  baseImagesDidChange: (->
    return unless App.get('baseImages.length') > 0
    unless @get 'selectedBaseImage'
      @set 'selectedBaseImage', App.get('baseImages.firstObject.repo')
  ).observes 'App.baseImages.@each'

  hostImagesDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('select[name=containers-new-option-host-image]')?.change()
  ).observes 'controller.selectedHost.images.@each'

  hostVolumesDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('select[name=containers-new-selected-volume]')?.change()
  ).observes 'controller.selectedHost.volumes.@each'

  didInsertElement: ->
    @_super()
    App.slug.slugify(@$('#containers-new-name'), @$('#containers-new-slug'))

    if App.get('baseImages.length') > 0 && !@get('selectedBaseImage')
      @set 'selectedBaseImage', App.get('baseImages.firstObject.repo')

    @$('input[name=option]').on('change.containers-new', (ev) =>
      if $(ev.target).is(':checked')
        @set 'selectedOption', ev.target.value
    ).change()

    @$('.option-input').on 'click.containers-new', (ev) =>
      Em.run.next this, ->
        @$("##{$(ev.target).attr('name')}").click()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#containers-new-name'))
    @$('.option-input').off('click.containers-new')
    @$('input[name=option]').off('change.containers-new')

  actions:
    openVolumeMounts: ->
      return if @get 'controller.saving'
      ui.showModal('#volume-mounts-dialog')

    openPortBindings: ->
      return if @get 'controller.saving'
      ui.showModal('#port-bindings-dialog')

    openEnvVars: ->
      return if @get 'controller.saving'
      ui.showModal('#env-vars-dialog')

    openExperimental: ->
      return if @get 'controller.saving'
      ui.showModal('#experimental-dialog')

    closeVolumeMounts: ->
      ui.hideModal('#volume-mounts-dialog')

    closePortBindings: ->
      ui.hideModal('#port-bindings-dialog')

    closeEnvVars: ->
      ui.hideModal('#env-vars-dialog')

    closeExperimental: ->
      ui.hideModal('#experimental-dialog')

    create: ->
      @get('controller').send 'create', @get('image')
