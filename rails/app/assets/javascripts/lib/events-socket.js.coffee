#= require ./socket/socket

App.EventsSocket = Lib.Socket.extend
  init: ->
    @_super.apply this, arguments
    @_addEventHandlers()

  connect: ->
    origin = location.origin.replace('http', 'ws')
    token = encodeURIComponent(App.get('token'))
    @set 'url', "#{origin}/api/v0/events?token=#{token}"
    @_super.apply this, arguments

  didOpen: (ev) ->
    @_super.apply this, arguments

  didReceiveMessage: (ev) ->
    msg = JSON.parse(ev.data)
    console.log msg.name, msg.data
    @trigger msg.name, msg.data
    @_super.apply this, arguments

  _addEventHandlers: ->
    ['container.created', 'container.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        findAllContainers()?.reload()

    ['container.started', 'container.stopped', 'container.restarted', 'container.updated'].forEach (evt) =>
      @on evt, (data) ->
        containerData = data.container
        findContainer(containerData)?.load(containerData)

    ['host.created', 'host.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        findAllHosts()?.reload()

    @on 'host.creating', (data) =>
      return unless host = findHostBySlug(data)
      if progress = parseProgress(data)
        host.set 'progress', progress

    @on 'host.create_failed', (data) ->
      findHostBySlug(data)?.set 'progress', null

    @on 'host.created', (data) =>
      hostData = data.host
      return unless host = findHostBySlug(hostData)
      host.load(hostData)
      host.set 'progress', null

    ['host.started', 'host.stopped', 'host.restarted', 'host.updated'].forEach (evt) =>
      @on evt, (data) ->
        hostData = data.host
        findHost(hostData)?.load(hostData)

    ['volume.created', 'volume.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        volumeData = data.volume
        findAllVolumes(volumeData)?.reload()

    ['registry.added', 'registry.removed'].forEach (evt) =>
      @on evt, (data) ->
        registryData = data.registry
        findAllRegistries(registryData)?.reload()

    ['image.created', 'image.removed', 'image.tagged'].forEach (evt) =>
      @on evt, (data) ->
        imageData = data.image
        findAllImages(imageData)?.reload()

    ['image.building', 'image.pulling'].forEach (evt) =>
      @on evt, (data) ->
        return unless image = findImage(data)
        if progress = parseProgress(data)
          image.set 'progress', progress

    @on 'image.build_failed', (data) ->
      findImage(data)?.set 'progress', null

    @on 'image.built', (data) ->
      imageData = data.image
      return unless image = findImage(imageData)
      image.load(imageData)
      image.set 'progress', null

    ['image.pulled', 'image.pull_failed'].forEach (evt) =>
      @on evt, (data) ->
        imageData = data.image
        return unless image = findImage(imageData)
        image.load(imageData)
        image.set 'updating', false
        image.set 'progress', null

    @on 'image.updated', (data) ->
      imageData = data.image
      findImage(imageData)?.load(imageData)

    @on 'image.pushed', (data) ->
      imageData = data.image
      return unless imageData && (image = findImage(imageData))
      image.load(imageData)
      image.set 'pushing', false

findAllContainers = ->
  App.get('containers')

findContainer = (data) ->
  pk = App.Container.primaryKey
  findAllContainers()?.findBy(pk, data[pk])

findAllHosts = ->
  App.get('hosts')

findHost = (data) ->
  pk = App.Host.primaryKey
  findAllHosts()?.findBy(pk, data[pk])

findHostBySlug = (data) ->
  App.Host._findInCache(data.slug)

findHostByHostId = (data) ->
  findAllHosts()?.findBy('id', data.host_id)

findAllImages = (data) ->
  findHostByHostId(data)?.get('images')

findImage = (data) ->
  pk = App.Image.primaryKey
  App.Image._findInCache(data[pk])

findAllVolumes = (data) ->
  findHostByHostId(data)?.get('volumes')

findAllRegistries = (data) ->
  findHostByHostId(data)?.get('registries')

parseProgress = (data) ->
  return unless progress = data.progress
  progress = JSON.parse(progress)
  if progress.status
    progress.status += "..."
  if progress.stream
    progress.status = progress.stream
  if bar = progress.progress
    eta = bar.match(/^\[.*\]\s+(.*)$/)?[1]
    id = progress.id
    if eta && id
      progress.status = "Downloading #{id}... #{eta}"
  progress
