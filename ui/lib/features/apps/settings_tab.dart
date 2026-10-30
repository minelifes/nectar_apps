import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';
import 'app_actions.dart';
import 'builds_tab.dart';
import 'resources.dart';
import 'volume_files_dialog.dart';

/// Edits the app settings (saved with PATCH; most take effect with the next deploy).
class SettingsTab extends ConsumerStatefulWidget {
  const SettingsTab({super.key, required this.app, required this.onDeployed});

  final App app;
  final void Function(Release?) onDeployed;

  @override
  ConsumerState<SettingsTab> createState() => _SettingsTabState();
}

class _Port {
  _Port(this.port, this.protocol, this.nodePort);

  final TextEditingController port;
  String protocol;
  final TextEditingController nodePort;
}

class _Vol {
  _Vol(this.name, this.mount, this.size, {this.existing = false, this.storageClass = '', this.shared = false});

  final TextEditingController name;
  final TextEditingController mount;
  final TextEditingController size;

  /// Fixed once the volume exists.
  String storageClass;
  bool shared;

  /// Saved before: the name is fixed and the size can only grow.
  final bool existing;
}

class _SettingsTabState extends ConsumerState<SettingsTab> {
  late final Map<String, dynamic> _cfg = jsonDecode(jsonEncode(widget.app.config)) as Map<String, dynamic>;
  late final _name = TextEditingController(text: widget.app.name);
  late String _version = widget.app.runtimeVersion;
  late final _image = TextEditingController(text: '${_cfg['image'] ?? ''}');
  late final _port = TextEditingController(text: '${_cfg['port'] ?? ''}');
  late String _healthType = const {'http', 'tcp', 'none'}.contains(_health['type']) ? '${_health['type']}' : 'tcp';
  late final _healthPath = TextEditingController(text: '${_health['path'] ?? ''}');
  late final _healthDelay = TextEditingController(text: _nz(_health['initial_delay_seconds']));
  late final _cpu = TextEditingController(text: '${_res['cpu_millicores'] ?? 250}');
  late final _mem = TextEditingController(text: '${_res['memory_mb'] ?? 256}');
  late bool _noLimit = _res['no_limit'] == true;
  late final _command = TextEditingController(text: _commandText(_cfg['command']));
  late final _args = TextEditingController(text: '${_cfg['args'] ?? ''}');
  late final _release = TextEditingController(text: '${_cfg['release_command'] ?? ''}');
  late final _schedule = TextEditingController(text: '${_cfg['schedule'] ?? ''}');
  late final List<_Port> _ports = [
    for (final p in (_cfg['expose'] as List? ?? const []))
      _Port(TextEditingController(text: '${(p as Map)['port']}'), '${p['protocol'] ?? 'TCP'}'.toUpperCase(), TextEditingController(text: _nz(p['node_port']))),
  ];
  late final List<_Vol> _vols = [
    for (final v in widget.app.volumes)
      _Vol(TextEditingController(text: v.name), TextEditingController(text: v.mountPath), TextEditingController(text: '${v.sizeMb}'),
          existing: true, storageClass: v.storageClass, shared: v.shared),
  ];
  bool _dirty = false;
  bool _saving = false;

  Map<String, dynamic> get _health => Map<String, dynamic>.from(_cfg['health'] as Map? ?? const {});
  Map<String, dynamic> get _res => Map<String, dynamic>.from(_cfg['resources'] as Map? ?? const {});

  static String _nz(Object? v) => v == null || v == 0 ? '' : '$v';

  /// Shows ["sh", "-c", X] as X and other commands joined by spaces.
  static String _commandText(Object? c) {
    final l = (c as List? ?? const []).map((e) => '$e').toList();
    if (l.length == 3 && l[0] == 'sh' && l[1] == '-c') return l[2];
    return l.join(' ');
  }

  void _changed() => setState(() => _dirty = true);

  @override
  void dispose() {
    for (final c in [_name, _image, _port, _healthPath, _healthDelay, _cpu, _mem, _command, _args, _release, _schedule]) {
      c.dispose();
    }
    for (final p in _ports) {
      p.port.dispose();
      p.nodePort.dispose();
    }
    for (final v in _vols) {
      v.name.dispose();
      v.mount.dispose();
      v.size.dispose();
    }
    super.dispose();
  }

  int? _int(TextEditingController c) => int.tryParse(c.text.trim());

  /// Builds the PATCH body; returns null after showing what is wrong.
  Map<String, dynamic>? _body() {
    final app = widget.app;
    String? err;
    final cfg = Map<String, dynamic>.from(_cfg);
    if (app.runtime == 'image') cfg['image'] = _image.text.trim();
    if (app.isWeb) {
      final port = _int(_port);
      if (port == null || port < 1 || port > 65535) err ??= 'Port must be 1–65535';
      cfg['port'] = port ?? 0;
      final delay = _healthDelay.text.trim().isEmpty ? 0 : _int(_healthDelay);
      if (delay == null || delay < 0) err ??= 'Initial delay must be a number of seconds';
      cfg['health'] = {
        'type': _healthType,
        if (_healthType == 'http') 'path': _healthPath.text.trim().isEmpty ? '/' : _healthPath.text.trim(),
        if ((delay ?? 0) > 0) 'initial_delay_seconds': delay,
      };
    }
    final cpu = _int(_cpu), mem = _int(_mem);
    if (cpu == null || cpu < 10) err ??= 'CPU must be at least 10 millicores';
    if (mem == null || mem < 16) err ??= 'Memory must be at least 16 MiB';
    cfg['resources'] = {'cpu_millicores': cpu ?? 0, 'memory_mb': mem ?? 0, if (_noLimit) 'no_limit': true};
    final cmd = _command.text.trim();
    cfg['command'] = cmd.isEmpty ? <String>[] : ['sh', '-c', cmd];
    cfg['args'] = _args.text.trim();
    cfg['release_command'] = _release.text.trim();
    if (app.kind == 'cron') cfg['schedule'] = _schedule.text.trim();
    if (app.hasReplicas) {
      cfg['expose'] = [
        for (final p in _ports)
          {'port': _int(p.port) ?? 0, 'protocol': p.protocol, if (_int(p.nodePort) != null) 'node_port': _int(p.nodePort)},
      ];
    }
    cfg['volumes'] = [
      for (final v in _vols)
        {
          'name': v.name.text.trim(),
          'mount_path': v.mount.text.trim(),
          'size_mb': _int(v.size) ?? 0,
          if (v.storageClass.isNotEmpty) 'storage_class': v.storageClass,
          if (v.shared) 'shared': true,
        },
    ];
    if (_name.text.trim().isEmpty) err ??= 'The name cannot be empty';
    if (err != null) {
      showSnack(context, err, error: true);
      return null;
    }
    return {
      'name': _name.text.trim(),
      if (app.runtime != 'image' && _version.isNotEmpty) 'runtime_version': _version,
      'config': cfg,
    };
  }

  Future<void> _save({required bool deploy}) async {
    final body = _body();
    if (body == null) return;
    setState(() => _saving = true);
    try {
      await ref.read(apiProvider).patch('/apps/${widget.app.id}', data: body);
      if (!mounted) return;
      setState(() => _dirty = false);
      if (deploy) {
        widget.onDeployed(await redeploy(context, ref, widget.app));
      } else {
        showSnack(context, 'Saved. Redeploy the app to apply.');
      }
      refreshApp(ref, widget.app.id);
      ref.invalidate(appVolumesProvider(widget.app.id));
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final app = widget.app;
    final session = ref.watch(sessionProvider);
    final canEdit = session.can('app.edit', projectId: app.projectId);
    final canDeploy = session.can('app.deploy', projectId: app.projectId);
    final canVolumes = session.can('volume.manage', projectId: app.projectId);
    final c = NectarColors.of(context);
    final runtimes = ref.watch(runtimesProvider);
    final rt = runtimes.hasValue ? runtimes.value!.where((r) => r.id == app.runtime).firstOrNull : null;

    Widget field(TextEditingController ctl, String label, {String? hint, String? help, double width = 260, bool mono = false, bool number = false, bool enabled = true}) =>
        SizedBox(
          width: width,
          child: TextField(
            controller: ctl,
            enabled: canEdit && enabled,
            keyboardType: number ? TextInputType.number : null,
            inputFormatters: number ? [FilteringTextInputFormatter.digitsOnly] : null,
            style: mono ? monoStyle(context, size: 13.5) : null,
            decoration: InputDecoration(labelText: label, hintText: hint, helperText: help, helperMaxLines: 2),
            onChanged: (_) => _changed(),
          ),
        );

    final sections = <Widget>[
      NCard(
        title: 'General',
        child: Wrap(
          spacing: 16,
          runSpacing: 12,
          children: [
            field(_name, 'Name'),
            if (app.runtime == 'image') field(_image, 'Image', hint: 'ghcr.io/acme/app:1.2.3', width: 420, mono: true),
            if (app.runtime == 'image' && canEdit && ref.watch(appSourceProvider(app.id)).value == null)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: TextButton.icon(
                  onPressed: () => showSourceDialog(context, appId: app.id),
                  icon: const Icon(Icons.merge_type_rounded, size: 18),
                  label: const Text('Build from a Git repository instead'),
                ),
              ),
            if (rt != null && !rt.takesImage && rt.versions.length > 1)
              SizedBox(
                width: 260,
                child: DropdownButtonFormField<String>(
                  initialValue: rt.versions.any((v) => v.id == _version) ? _version : null,
                  decoration: const InputDecoration(labelText: 'Version'),
                  items: [for (final v in rt.versions) DropdownMenuItem(value: v.id, child: Text(v.label))],
                  onChanged: canEdit ? (v) => setState(() {
                        _version = v ?? _version;
                        _dirty = true;
                      }) : null,
                ),
              ),
            if (app.kind == 'cron') field(_schedule, 'Schedule', hint: '*/15 * * * *', help: 'Cron syntax, server time (UTC)', mono: true),
          ],
        ),
      ),
      if (app.isWeb)
        NCard(
          title: 'Network and health check',
          child: Wrap(
            spacing: 16,
            runSpacing: 12,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              field(_port, 'Port the app listens on', width: 200, number: true),
              SegmentedButton<String>(
                segments: const [
                  ButtonSegment(value: 'http', label: Text('HTTP')),
                  ButtonSegment(value: 'tcp', label: Text('Port open')),
                  ButtonSegment(value: 'none', label: Text('None')),
                ],
                selected: {_healthType},
                onSelectionChanged: canEdit ? (s) => setState(() {
                      _healthType = s.first;
                      _dirty = true;
                    }) : null,
              ),
              if (_healthType == 'http') field(_healthPath, 'Health path', hint: '/health', width: 220, mono: true),
              if (_healthType != 'none')
                field(_healthDelay, 'Initial delay (s)', width: 160, number: true, help: 'Wait before the first check'),
            ],
          ),
        ),
      NCard(
        title: 'Resources per replica',
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final p in resourcePresets)
                  ChoiceChip(
                    label: Text('${p.label} · ${formatCpu(p.cpuMillicores)} · ${formatMb(p.memoryMb)}'),
                    selected: _int(_cpu) == p.cpuMillicores && _int(_mem) == p.memoryMb,
                    onSelected: canEdit
                        ? (_) => setState(() {
                              _cpu.text = '${p.cpuMillicores}';
                              _mem.text = '${p.memoryMb}';
                              _dirty = true;
                            })
                        : null,
                  ),
              ],
            ),
            const SizedBox(height: 12),
            Wrap(
              spacing: 16,
              runSpacing: 12,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                field(_cpu, 'CPU (millicores)', width: 180, number: true, help: '1000 = one core'),
                field(_mem, 'Memory (MiB)', width: 180, number: true),
                SizedBox(
                  width: 320,
                  child: SwitchListTile(
                    contentPadding: EdgeInsets.zero,
                    value: _noLimit,
                    onChanged: canEdit ? (v) => setState(() {
                          _noLimit = v;
                          _dirty = true;
                        }) : null,
                    title: const Text('No limit'),
                    subtitle: Text('Reserve these, but allow using any free capacity', style: TextStyle(color: c.muted, fontSize: 12)),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
      NCard(
        title: 'Commands',
        child: Wrap(
          spacing: 16,
          runSpacing: 12,
          children: [
            field(_command, 'Start command', hint: 'Leave empty for the default', width: 520, mono: true, help: 'Runs with sh -c, so the image needs a shell'),
            field(_args, 'Arguments (APP_ARGS)', width: 520, mono: true, help: 'Passed to the default start command of the runtime'),
            field(_release, 'Release command', hint: 'e.g. ./migrate up', width: 520, mono: true, help: 'Runs once before each deploy goes live; a failure stops the deploy'),
          ],
        ),
      ),
      if (app.hasReplicas) _portsCard(context, canEdit),
      _volumesCard(context, canEdit && canVolumes, canVolumes),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final s in sections) ...[s, const SizedBox(height: NectarTokens.gap)],
        if (canEdit)
          Wrap(
            alignment: WrapAlignment.end,
            spacing: 8,
            runSpacing: 8,
            children: [
              OutlinedButton(onPressed: !_dirty || _saving ? null : () => _save(deploy: false), child: const Text('Save')),
              if (canDeploy)
                FilledButton.icon(
                  onPressed: !_dirty || _saving ? null : () => _save(deploy: true),
                  icon: const Icon(Icons.rocket_launch_rounded, size: 18),
                  label: Text(_saving ? 'Saving…' : 'Save and redeploy'),
                ),
            ],
          )
        else
          Text('Changing settings needs the app.edit permission.', style: TextStyle(color: c.muted, fontSize: 12.5)),
      ],
    );
  }

  Widget _portsCard(BuildContext context, bool canEdit) {
    final c = NectarColors.of(context);
    return NCard(
      title: 'Open ports on the server',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text('TCP/UDP ports reachable from outside (MQTT, game servers…). Web traffic goes through domains instead.',
              style: TextStyle(color: c.muted, fontSize: 12.5)),
          const SizedBox(height: 8),
          for (final p in _ports)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Row(
                children: [
                  SizedBox(
                    width: 130,
                    child: TextField(
                      controller: p.port,
                      enabled: canEdit,
                      keyboardType: TextInputType.number,
                      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                      decoration: const InputDecoration(labelText: 'Container port', isDense: true),
                      onChanged: (_) => _changed(),
                    ),
                  ),
                  const SizedBox(width: 8),
                  DropdownButton<String>(
                    value: p.protocol == 'UDP' ? 'UDP' : 'TCP',
                    items: const [DropdownMenuItem(value: 'TCP', child: Text('TCP')), DropdownMenuItem(value: 'UDP', child: Text('UDP'))],
                    onChanged: canEdit ? (v) => setState(() {
                          p.protocol = v ?? 'TCP';
                          _dirty = true;
                        }) : null,
                  ),
                  const SizedBox(width: 8),
                  SizedBox(
                    width: 170,
                    child: TextField(
                      controller: p.nodePort,
                      enabled: canEdit,
                      keyboardType: TextInputType.number,
                      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                      decoration: const InputDecoration(labelText: 'Server port', hintText: 'auto (30000–32767)', isDense: true),
                      onChanged: (_) => _changed(),
                    ),
                  ),
                  if (canEdit)
                    IconButton(
                      tooltip: 'Remove',
                      icon: const Icon(Icons.close_rounded, size: 18),
                      onPressed: () => setState(() {
                        _ports.remove(p);
                        _dirty = true;
                      }),
                    ),
                ],
              ),
            ),
          if (canEdit)
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                onPressed: () => setState(() {
                  _ports.add(_Port(TextEditingController(), 'TCP', TextEditingController()));
                  _dirty = true;
                }),
                icon: const Icon(Icons.add_rounded, size: 18),
                label: const Text('Open a port'),
              ),
            ),
        ],
      ),
    );
  }

  Widget _volumesCard(BuildContext context, bool canEdit, bool canDelete) {
    final c = NectarColors.of(context);
    final live = ref.watch(appVolumesProvider(widget.app.id));
    final infos = live.hasValue ? {for (final v in live.value!) v.name: v} : const <String, VolumeInfo>{};
    final orphans = live.hasValue ? live.value!.where((v) => v.orphaned).toList() : const <VolumeInfo>[];
    final classes = ref.watch(storageClassesProvider).value ?? const <StorageClassInfo>[];
    return NCard(
      title: 'Volumes',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text('Persistent disks: files written there survive restarts and deploys. Everything else in the container is temporary.',
              style: TextStyle(color: c.muted, fontSize: 12.5)),
          const SizedBox(height: 8),
          for (final v in _vols) ...[
            Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: Row(
                children: [
                  SizedBox(
                    width: 140,
                    child: TextField(
                      controller: v.name,
                      enabled: canEdit && !v.existing,
                      decoration: const InputDecoration(labelText: 'Name', hintText: 'data', isDense: true),
                      onChanged: (_) => _changed(),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: TextField(
                      controller: v.mount,
                      enabled: canEdit,
                      style: monoStyle(context, size: 13.5),
                      decoration: const InputDecoration(labelText: 'Mounted at', hintText: '/data', isDense: true),
                      onChanged: (_) => _changed(),
                    ),
                  ),
                  const SizedBox(width: 8),
                  SizedBox(
                    width: 120,
                    child: TextField(
                      controller: v.size,
                      enabled: canEdit,
                      keyboardType: TextInputType.number,
                      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                      decoration: const InputDecoration(labelText: 'Size (MiB)', isDense: true),
                      onChanged: (_) => _changed(),
                    ),
                  ),
                  if (canEdit)
                    IconButton(
                      tooltip: 'Remove from the app (the data is kept until you delete it)',
                      icon: const Icon(Icons.close_rounded, size: 18),
                      onPressed: () => setState(() {
                        _vols.remove(v);
                        _dirty = true;
                      }),
                    ),
                ],
              ),
            ),
            _volumeStorage(context, v, canEdit, classes),
            if (infos[v.name.text.trim()] case final info? when info.usedBytes > 0 && info.requestedMb > 0)
              Padding(
                padding: const EdgeInsets.only(bottom: 4),
                child: UsageBar(
                  label: 'Used',
                  value: info.usedBytes / (info.requestedMb * 1024 * 1024),
                  trailing: '${formatBytes(info.usedBytes)} of ${formatMb(info.requestedMb)}',
                ),
              ),
            if (infos[v.name.text.trim()] case final info? when info.phase.isNotEmpty && canDelete)
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton.icon(
                  onPressed: () => showVolumeFiles(context, appId: widget.app.id, volume: info.name),
                  icon: const Icon(Icons.folder_open_rounded, size: 18),
                  label: const Text('Files'),
                ),
              ),
            const SizedBox(height: 6),
          ],
          if (canEdit)
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                onPressed: () => setState(() {
                  _vols.add(_Vol(TextEditingController(text: _vols.isEmpty ? 'data' : ''), TextEditingController(text: _vols.isEmpty ? '/data' : ''),
                      TextEditingController(text: '1024')));
                  _dirty = true;
                }),
                icon: const Icon(Icons.add_rounded, size: 18),
                label: const Text('Add volume'),
              ),
            ),
          if (!canEdit && widget.app.volumes.isEmpty) Text('No volumes.', style: TextStyle(color: c.muted, fontSize: 13)),
          if (orphans.isNotEmpty) ...[
            const Divider(height: 24),
            Text('Removed from the app, data still kept', style: Theme.of(context).textTheme.titleSmall),
            for (final o in orphans)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: Icon(Icons.inventory_2_outlined, color: c.muted),
                title: Text(o.name),
                subtitle: Text('${o.claim} · ${formatMb(o.requestedMb)}', style: TextStyle(color: c.muted, fontSize: 12)),
                trailing: canDelete
                    ? TextButton(
                        style: TextButton.styleFrom(foregroundColor: c.failed),
                        onPressed: () => _deleteVolume(o),
                        child: const Text('Delete data'),
                      )
                    : null,
              ),
          ],
        ],
      ),
    );
  }

  /// Storage class and sharing of a volume: chosen for new volumes, shown for existing ones.
  Widget _volumeStorage(BuildContext context, _Vol v, bool canEdit, List<StorageClassInfo> classes) {
    final c = NectarColors.of(context);
    if (v.existing || !canEdit) {
      final cls = v.storageClass.isEmpty ? 'default storage' : v.storageClass;
      return Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Text('$cls${v.shared ? ' · shared' : ''}', style: TextStyle(color: c.muted, fontSize: 12)),
      );
    }
    if (classes.length < 2) return const SizedBox.shrink(); // only local-path: nothing to choose
    final chosen = classes.where((x) => x.name == v.storageClass).firstOrNull;
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Wrap(
        spacing: 12,
        runSpacing: 6,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          SizedBox(
            width: 260,
            child: DropdownButtonFormField<String>(
              initialValue: v.storageClass,
              isExpanded: true,
              decoration: const InputDecoration(labelText: 'Storage', isDense: true),
              items: [
                const DropdownMenuItem(value: '', child: Text('Default')),
                for (final x in classes) DropdownMenuItem(value: x.name, child: Text('${x.name}${x.description.isEmpty ? '' : ' — ${x.description}'}', overflow: TextOverflow.ellipsis)),
              ],
              onChanged: (val) => setState(() {
                v.storageClass = val ?? '';
                if (!(classes.where((x) => x.name == v.storageClass).firstOrNull?.shared ?? false)) v.shared = false;
                _dirty = true;
              }),
            ),
          ),
          if (chosen?.shared ?? false)
            Row(mainAxisSize: MainAxisSize.min, children: [
              Checkbox(
                value: v.shared,
                onChanged: (val) => setState(() {
                  v.shared = val ?? false;
                  _dirty = true;
                }),
              ),
              const Text('Shared by all replicas', style: TextStyle(fontSize: 13)),
            ]),
          Text('Fixed once the volume exists.', style: TextStyle(color: c.muted, fontSize: 12)),
        ],
      ),
    );
  }

  Future<void> _deleteVolume(VolumeInfo v) async {
    final ok = await confirmAction(context, 'Delete the data of ${v.name}?', 'Everything stored on this volume is deleted for good.', 'Delete data', danger: true);
    if (!ok || !mounted) return;
    try {
      await ref.read(apiProvider).delete('/apps/${widget.app.id}/volumes/${v.name}');
      ref.invalidate(appVolumesProvider(widget.app.id));
      if (mounted) showSnack(context, 'Volume ${v.name} deleted');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }
}
