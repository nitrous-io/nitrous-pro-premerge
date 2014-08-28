App.slug =
  toSlug: (text) ->
    (text.dasherize().replace(/[^A-Za-z0-9\-]/g, '-') + '-' + util.randomInteger(10, 99).toString()).replace(/^-+/, '')

  slugify: ($nameField, $slugField) ->
    $nameField.on 'input.slugify', (ev) =>
      $slugField.val(@toSlug($.trim(ev.target.value))).change()

  unslugify: ($nameField) ->
    $nameField.off 'input.slugify'

  inputPattern: '[a-z0-9][a-z0-9\-]*'
  inputTitle: 'Slug may only contain alphanumeric characters or dashes and cannot begin with a dash.'
