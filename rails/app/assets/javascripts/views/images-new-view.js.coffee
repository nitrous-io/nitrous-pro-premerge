window.Blob ?= window.WebKitBlob

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
      @get('controller.name') && @get('controller.selectedHostSlug') && (
        (@get('selectedOption') == 'file' && @get('selectedFile')) ||
        (@get('selectedOption') == 'code' && @get('dockerfileCode'))
      )
    )
  ).property 'controller.name', 'controller.selectedHostSlug', 'selectedOption', 'selectedFile', 'dockerfileCode'

  didInsertElement: ->
    @_super()
    @set 'codeMirror', codemirror = CodeMirror.fromTextArea @$('#dockerfile-code')[0],
      lineNumbers: true
      viewportMargin: Infinity
    codemirror.changeHandler = =>
      @set 'dockerfileCode', codemirror.getValue()
    codemirror.focusHandler = =>
      @$('#images-new-option-code').click()
    codemirror.on 'change', codemirror.changeHandler
    codemirror.on 'focus', codemirror.focusHandler

    @$('input[name=option]').on('change.images-new', (ev) =>
      if $(ev.target).is(':checked')
        @set 'selectedOption', ev.target.value
    ).change()

    @$('#images-new-name').focus()

  willDestroyElement: ->
    @_super()
    if codeMirror = @get('codeMirror')
      codeMirror.off 'change', codeMirror.changeHandler
      codeMirror.off 'focus', codeMirror.focusHandler
      codeMirror.changeHandler = null
      codeMirror.focusHandler = null
      codeMirror.toTextArea()
    @set 'codeMirror', null
    @$('input[name=option]').off('change.images-new')

  actions:
    didChangeFile: ->
      if files = @$('#images-new-file')[0].files
        file = files[0]
        @set 'selectedFile', file
        @set 'selectedFileName', file?.name
        @$('#images-new-option-file').click()

    create: ->
      file = switch @get('selectedOption')
        when 'file'
          @get 'selectedFile'
        when 'code'
          new Blob([@get 'dockerfileCode'], type: 'text/plain')
      @get('controller').send 'create', file
