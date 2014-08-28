App.HostsNewController = Em.ObjectController.extend
  name: null
  slug: null
  selectedRegionSlug: null
  selectedPlanSlug: null
  error: null
  invalidField: null
  saving: false

  clear: ->
    @setProperties
      name: null
      slug: null
      selectedRegionSlug: null
      selectedPlanSlug: null
      error: null
      invalidField: null
      saving: false

  selectedPlan: (->
    if slug = @get('selectedPlanSlug')
      App.get('plans').findBy('slug', slug)
  ).property 'selectedPlanSlug', 'App.plans.@each'

  actions:
    cancel: ->
      @transitionToRoute 'home.index'
      false

    create: ->
      @set 'saving', true
      host = App.Host.create
        regionSlug: @get('selectedRegionSlug')
        planSlug: @get('selectedPlanSlug')
        name: @get('name')
        slug: @get('slug')
      host.save().then =>
        App.get('hosts').reload().then =>
          @transitionToRoute 'host.index', host.get 'slug'
          @set 'saving', false
      , (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
          App.snackbar.post(errMsg.capitalize())
        @set 'saving', false
