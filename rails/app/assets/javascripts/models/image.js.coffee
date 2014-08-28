#= require ./model
attr = SM.attr

App.Image = klass = App.Model.extend
  id: attr(serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  name: attr()
  tag: attr()
  actualSize: attr('number', as: 'actual_size')
  virtualSize: attr('number', as: 'virtual_size')
  createdAt: attr('date', as: 'created_at', serialize: false)

  nameTagAndId: (->
    name = @get 'name'
    tag = @get 'tag'
    id = @get 'id'
    "#{name}:#{tag} (#{id})"
  ).property 'name', 'tag', 'id'

klass.reopenClass
  url: '/hosts/:hostId/images'
  primaryKey: 'id'
  rootKey: 'image'
  collectionKey: 'images'
