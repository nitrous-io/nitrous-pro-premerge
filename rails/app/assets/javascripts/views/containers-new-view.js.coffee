App.ContainersNewView = Em.View.extend
  elementId: 'containers-new-view'
  classNames: ['ui', 'card']

  showAdvanced: false

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  createButtonDisabled: (->
    true
  ).property()

  didInsertElement: ->
    @_super()
    App.slug.slugify(@$('#containers-new-name'), @$('#containers-new-slug'))

    @$('#containers-new-name').focus()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#containers-new-name'))

  actions:
    toggleShowAdvanced: ->
      @$('#toggle-advanced-btn').blur()
      @toggleProperty 'showAdvanced'
      false
