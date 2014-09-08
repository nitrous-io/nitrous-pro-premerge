Em.Handlebars.helper 'ago', (date) ->
  if date instanceof Date && date.valueOf() > 0
    $.timeago(date)

Em.Handlebars.helper 'utc', (date) ->
  if date instanceof Date && date.valueOf() > 0
    date.toUTCString()

Em.Handlebars.helper 'dateabbr', (date) ->
  if date instanceof Date && date.valueOf() > 0
    new Ember.Handlebars.SafeString(Em.$('<abbr>').attr('title', date.toString()).text($.timeago(date))[0].outerHTML)
