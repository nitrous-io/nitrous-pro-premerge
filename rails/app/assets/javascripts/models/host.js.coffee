#= require ./model
attr = SM.attr

App.Host = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  regionId: attr('number', as: 'region_id', serialize: false)
  planId: attr('number', as: 'plan_id', serialize: false)
  slug: attr()
  name: attr()
  state: attr(serialize: false)
  ipAddress: attr(as: 'ip_address', serialize: false)
  freighterdPort: attr('number', as: 'freighterd_port', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  terminatedAt: attr('date', as: 'terminated_at', serialize: false)

  # only used in create
  planSlug: attr(as: 'size')
  regionSlug: attr(as: 'region')

  stats: null

  isRunning: Em.computed.equal 'state', 'running'
  isStopped: Em.computed.equal 'state', 'stopped'
  isTerminated: Em.computed.equal 'state', 'terminated'

  displayName: (->
    @get('name') || @get('slug')
  ).property 'name', 'slug'

  nameAndSlug: (->
    name = @get 'name'
    slug = @get 'slug'
    if name && slug
      "#{name} (#{slug})"
    else
      slug
  ).property 'name', 'slug'

  containers: (->
    if id = @get('id')
      App.get('containers')?.filterBy('hostId', id)
  ).property 'id', 'App.containers.@each'

  registries: (->
    if id = @get('id')
      App.Registry.fetch(hostId: id).value
  ).property 'id'

  volumes: (->
    if id = @get('id')
      App.Volume.fetch(hostId: id).value
  ).property 'id'

  images: (->
    if id = @get('id')
      App.Image.fetch(hostId: id).value
  ).property 'id'

  region: (->
    if regionId = @get('regionId')
      App.get('regions').findBy('id', regionId)
  ).property 'regionId', 'App.regions.@each'

  plan: (->
    if planId = @get('planId')
      App.get('plans').findBy('id', planId)
  ).property 'planId', 'App.plans.@each'

  selected: (->
    App.get('selectedHost') == this
  ).property 'App.selectedHost'

  hostStateDidChange: (->
    if @get('state') == 'running'
      Em.run.later(this, ->
        @fetchStats()
      , 1000)
    else
      @set 'stats', null
  ).observes 'state'

  fetchStats: ->
    if @get('state') == 'running'
      @ajax('GET', 'stats', period: '1s').then (data) =>
        if stats = data.stats
          @set 'stats', App.HostStats.create(stats)

klass.reopenClass
  url: '/hosts'
  primaryKey: 'slug'
  rootKey: 'host'
  collectionKey: 'hosts'
