App.ContainersNewController = Em.Controller.extend
  selectedHostSlug: null
  selectedImageId: null

  selectedHost: (->
    if slug = @get 'selectedHostSlug'
      App.get('hosts').findBy 'slug', slug
  ).property 'selectedHostSlug', 'App.hosts.@each'
