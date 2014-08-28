window.util =
  escapeRegex: (text) ->
    text.replace(/([.*+?^${}()|\[\]\/\\])/g, "\\$1")

  randomInteger: (min, max) ->
    Math.floor(Math.random() * ((max + 1) - min)) + min
