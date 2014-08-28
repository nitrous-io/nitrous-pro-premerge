App.HostRegistriesNewView = Em.View.extend
  elementId: 'host-registries-new-view'

  createButtonDisabled: (->
    !(@get('controller.url') && @get('controller.username') && @get('controller.email') && @get('controller.password'))
  ).property 'controller.url', 'controller.username', 'controller.email', 'controller.password'

  didInsertElement: ->
    @_super()
    $urlField = @$('#registries-new-url')

    @$('.registry-buttons .btn').on 'click', ->
      $this = $(this)
      $urlField.val($this.data('url')).change()
      $this.blur()
      false

    @$('.registry-buttons .btn.default').click()
    @$('#registries-new-username').focus()

  willDestroyElement: ->
    @_super()
    @$('.registry-buttons .btn').off()
