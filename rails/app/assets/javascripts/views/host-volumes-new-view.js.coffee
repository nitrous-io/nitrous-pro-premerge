App.HostVolumesNewView = Em.View.extend
  elementId: 'host-volumes-new-view'

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug'))
  ).property 'controller.name', 'controller.slug'

  didInsertElement: ->
    @_super()
    App.slug.slugify(@$('#volumes-new-name'), @$('#volumes-new-slug'))
    @$('#volumes-new-name').focus()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#volumes-new-name'))
