App.ContainersNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Container'

  renderTemplate: ->
    @render 'containers/new', outlet: 'content'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
