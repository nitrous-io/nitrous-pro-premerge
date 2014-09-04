#= require jquery
#= require webfonts
#= require_self
#= require ua-parser

window.pages =
  kit:
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
      $('.mac').removeClass('active')
      $('.win').removeClass('active')
      $('.lin').removeClass('active')
      if location.hash == '#apple'
        $('.mac').addClass('active')
        $('.windows').hide()
        $('.linux').hide()
        $('.apple').show()
      else if location.hash == '#windows'
        $('.win').addClass('active')
        $('.apple').hide()
        $('.linux').hide()
        $('.windows').show()
      else
        $('.lin').addClass('active')
        $('.apple').hide()
        $('.windows').hide()
        $('.linux').show()

  team:
    initialize: ->
      $("[rel='tooltip']").tooltip(html: true)
