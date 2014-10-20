Blob = window.Blob || window.WebKitBlob

App.ImagesNewView = Em.View.extend
  elementId: 'images-new-view'
  classNames: ['ui', 'card']

  codeMirror: null

  selectedOption: null
  selectedFile: null
  selectedFileName: null
  dockerfileCode: null

  createButtonDisabled: (->
    !(
      @get('controller.name') && @get('controller.slug') && @get('controller.selectedHostSlug') && (
        (@get('selectedOption') == 'remote' && @get('controller.remoteURL')) ||
        (@get('selectedOption') == 'file' && @get('selectedFile')) ||
        (@get('selectedOption') == 'code' && @get('dockerfileCode'))
      )
    )
  ).property 'controller.name', 'controller.slug', 'controller.selectedHostSlug', 'selectedOption', 'selectedFile', 'dockerfileCode', 'controller.remoteURL'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.image.progress.progressDetail.start')

  hostsDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      slug = @get('controller.preselectHost')
      if slug && !@get('selectedHostSlug')
        @set 'controller.selectedHostSlug', slug
      @$('select[name=host]')?.change()
  ).observes 'App.hosts.@each'

  preselectHostDidChange: (->
    if slug = @get('controller.preselectHost')
      @set 'controller.selectedHostSlug', slug
  ).observes 'controller.preselectHost'

  savingDidChange: (->
    return unless codeMirror = @get 'codeMirror'
    if @get 'controller.saving'
      codeMirror.setOption('readOnly', 'nocursor')
    else
      codeMirror.setOption('readOnly', false)
  ).observes 'controller.saving'

  didInsertElement: ->
    @_super()
    @set 'codeMirror', codeMirror = CodeMirror.fromTextArea @$('#dockerfile-code')[0],
      lineNumbers: true
      viewportMargin: Infinity

    codeMirror.changeHandler = =>
      @set 'dockerfileCode', codeMirror.getValue()
    codeMirror.on 'change', codeMirror.changeHandler

    @$('.editor .CodeMirror').on 'click.images-new', (ev) =>
      Em.run.next this, ->
        @$('#images-new-option-code').click()

    @$('#images-new-file').on 'click.images-new', (ev) =>
      Em.run.next this, ->
        @$('#images-new-option-file').click()

    @$('input[name=option]').on('change.images-new', (ev) =>
      if $(ev.target).is(':checked')
        @set 'selectedOption', ev.target.value
    ).change()

    App.slug.slugify(@$('#images-new-name'), @$('#images-new-slug'), false)

    @hostsDidChange()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#images-new-name'))
    @$('input[name=option]').off('change.images-new')
    @$('#images-new-file').off('click.images-new')
    @$('.editor .CodeMirror').off('click.images-new')
    if codeMirror = @get('codeMirror')
      codeMirror.off 'change', codeMirror.changeHandler
      codeMirror.changeHandler = null
      codeMirror.toTextArea()
    @set 'codeMirror', null

  actions:
    didChangeFile: ->
      if files = @$('#images-new-file')[0].files
        file = files[0]
        @set 'selectedFile', file
        @set 'selectedFileName', file?.name

    create: ->
      file = switch @get('selectedOption')
        when 'file'
          @get 'selectedFile'
        when 'code'
          new Blob([@get 'dockerfileCode'], type: 'text/plain')
      @get('controller').send 'create', file
