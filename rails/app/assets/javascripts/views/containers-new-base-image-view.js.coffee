App.ContainersNewBaseImageView = Em.View.extend
  templateName: 'containers/new-base-image'
  tagName: 'li'
  classNames: 'containers-new-base-image-view'
  classNameBindings: ['selected', 'disabled']

  baseImage: null
  selectedBaseImage: null

  selected: (->
    @get('selectedBaseImage') == @get('baseImage.repo')
  ).property 'selectedBaseImage', 'baseImage.repo'

  baseImageIconCss: (->
    makeBackgroundImageCss(@get 'baseImage.iconUrl')
  ).property 'baseImage.iconUrl'

  baseImageDefaultIconCss: (->
    makeBackgroundImageCss(App.BaseImage.defaultIconUrl)
  ).property 'App.BaseImage.defaultIconUrl'

  click: ->
    return if @get 'disabled'
    @set('selectedBaseImage', @get('baseImage.repo'))

makeBackgroundImageCss = (url) ->
  return unless url
  "background-image: url(#{url})"
