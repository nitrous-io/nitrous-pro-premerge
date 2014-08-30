#= require jquery
#= require webfonts
#= require_self
#= require ua-parser

window.App ||= {}

App =
  initialize: ->

    $(window).on 'hashchange', =>
      @showHidePlatform()

    if !(location.hash == '#windows' || location.hash == "#apple" || location.hash == '#linux')
      os = new UAParser().getOS().name
      if os.indexOf('Mac') != -1 || os.indexOf('OS X') != -1
        location.hash = '#apple'
      else if os.indexOf('Windows') != -1
        location.hash = '#windows'
      else
        location.hash = '#linux'

    @showHidePlatform()

  showHidePlatform: ->
    if location.hash == '#apple'
      $('.windows').hide()
      $('.linux').hide()
      $('.apple').show()
    else if location.hash == '#windows'
      $('.apple').hide()
      $('.linux').hide()
      $('.windows').show()
    else
      $('.apple').hide()
      $('.windows').hide()
      $('.linux').show()
