#= require ./model
attr = SM.attr

iconBucketUrl = "#{location.protocol}//nitrous-image-icons.s3.amazonaws.com"

App.BaseImage = klass = App.Model.extend
  id: attr(serialize: false)
  name: attr()
  slug: attr()
  repo: attr()
  description: attr()

  iconUrl: (->
    return unless slug = @get('slug')
    "#{iconBucketUrl}/#{slug}.png"
  ).property 'slug'

klass.reopenClass
  url: '/base_images'
  primaryKey: 'id'
  collectionKey: 'base_images'

  defaultIconUrl: "#{iconBucketUrl}/nitrous.png"
