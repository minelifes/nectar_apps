import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/hex.dart';
import '../auth/auth_frame.dart';
import '../projects/new_project_dialog.dart';
import 'artifact_file.dart';
import 'builds_tab.dart';
import 'deploy_progress.dart';
import 'env_editor.dart';
import 'resources.dart';

enum WizardStep { project, type, version, name, artifact, resources, settings, review }

extension on WizardStep {
  String get title => switch (this) {
        WizardStep.project => 'Project',
        WizardStep.type => 'App type',
        WizardStep.version => 'Version',
        WizardStep.name => 'Name & domain',
        WizardStep.artifact => 'Upload',
        WizardStep.resources => 'Resources',
        WizardStep.settings => 'Environment',
        WizardStep.review => 'Review & deploy',
      };
}

final _imageRe = RegExp(r'^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,254}$');

/// New App wizard: project → type → version (or image) → name/domain (creates the app) → upload → resources →
/// environment & health → review → deploy with live progress.
class NewAppPage extends ConsumerStatefulWidget {
  const NewAppPage({super.key, this.projectId});

  /// Preselected project (from the project page).
  final int? projectId;

  @override
  ConsumerState<NewAppPage> createState() => _NewAppPageState();
}

class _NewAppPageState extends ConsumerState<NewAppPage> {
  WizardStep _step = WizardStep.project;
  bool _busy = false;
  String? _error;

  // choices
  int? _projectId;
  Runtime? _runtime;
  String _kind = 'web';
  String? _version;
  final _image = TextEditingController();
  // image apps can be built from Git instead (needs the built-in registry)
  bool _fromGit = false;
  final _repo = TextEditingController();
  final _branch = TextEditingController(text: 'main');
  final _contextDir = TextEditingController();
  final _dockerfile = TextEditingController(text: 'Dockerfile');
  final _gitToken = TextEditingController();
  final _name = TextEditingController();
  final _slug = TextEditingController();
  bool _slugEdited = false;
  final _port = TextEditingController();
  final _schedule = TextEditingController(text: '*/15 * * * *');

  // the app is created when leaving the name step
  App? _app;

  // upload
  PickedArtifact? _file;
  Artifact? _artifact;
  double? _uploadProgress;
  CancelToken? _cancel;
  String? _uploadError;

  // resources
  int _cpu = 250;
  int _memory = 256;
  int _replicas = 1;
  bool _customResources = false;

  // environment & health
  final List<EnvVar> _env = [];
  String _healthType = 'tcp';
  final _healthPath = TextEditingController(text: '/');
  final _args = TextEditingController();

  // deploy
  Release? _release;
  bool? _deployOk;
  bool _forceOffered = false;

  @override
  void initState() {
    super.initState();
    _projectId = widget.projectId;
    _step = _projectId == null ? WizardStep.project : WizardStep.type;
    _name.addListener(() {
      if (!_slugEdited && _app == null) _slug.text = slugify(_name.text);
    });
  }

  @override
  void dispose() {
    _cancel?.cancel();
    for (final c in [_image, _name, _slug, _port, _schedule, _healthPath, _args, _repo, _branch, _contextDir, _dockerfile, _gitToken]) {
      c.dispose();
    }
    super.dispose();
  }

  List<WizardStep> get _steps => [
        if (widget.projectId == null) WizardStep.project,
        WizardStep.type,
        WizardStep.version,
        WizardStep.name,
        if (!(_runtime?.takesImage ?? false)) WizardStep.artifact,
        WizardStep.resources,
        WizardStep.settings,
        WizardStep.review,
      ];

  bool get _deploying => _release != null;

  // ------------------------------------------------------------------ actions --

  void _go(WizardStep s) => setState(() {
        _step = s;
        _error = null;
      });

  void _back() {
    final i = _steps.indexOf(_step);
    if (i > 0) _go(_steps[i - 1]);
  }

  Future<void> _next() async {
    final problem = _validate(_step);
    if (problem != null) {
      setState(() => _error = problem);
      return;
    }
    if (_step == WizardStep.name) {
      final ok = await _saveApp();
      if (!ok) return;
    }
    final i = _steps.indexOf(_step);
    if (i < _steps.length - 1) _go(_steps[i + 1]);
  }

  String? _validate(WizardStep s) {
    final rt = _runtime;
    switch (s) {
      case WizardStep.project:
        return _projectId == null ? 'Choose a project.' : null;
      case WizardStep.type:
        return rt == null ? 'Choose an app type.' : null;
      case WizardStep.version:
        if (rt!.takesImage && _fromGit) {
          final r = _repo.text.trim();
          if (!(r.startsWith('https://') || r.startsWith('http://') || r.startsWith('git@'))) return 'Enter the repository URL, e.g. https://github.com/acme/api.git';
          return _branch.text.trim().isEmpty ? 'Enter the branch to build.' : null;
        }
        if (rt.takesImage) return _imageRe.hasMatch(_image.text.trim()) ? null : 'Enter an image, e.g. ghcr.io/acme/shop:1.4.2';
        return _version == null ? 'Choose a version.' : null;
      case WizardStep.name:
        if (_name.text.trim().isEmpty) return 'Enter a name.';
        if (!RegExp(r'^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$').hasMatch(_slug.text.trim())) {
          return 'The slug must be 1–30 lowercase letters, digits or dashes.';
        }
        if (_kind == 'web') {
          final port = int.tryParse(_port.text.trim()) ?? 0;
          if (port < 1 || port > 65535) return 'The port must be 1–65535.';
        }
        if (_kind == 'cron' && _schedule.text.trim().split(RegExp(r'\s+')).length != 5 && !_schedule.text.trim().startsWith('@')) {
          return 'The schedule needs 5 cron fields, e.g. */15 * * * *';
        }
        return null;
      case WizardStep.artifact:
        if (_uploadProgress != null && _artifact == null) return 'Wait for the upload to finish.';
        return _artifact == null ? 'Upload your ${rt!.accept.join(' / ')} file.' : null;
      case WizardStep.resources:
        if (_cpu < 50 || _cpu > 64000) return 'CPU must be between 50m and 64 cores.';
        if (_memory < 32 || _memory > 262144) return 'Memory must be between 32 MB and 256 GB.';
        return null;
      case WizardStep.settings:
        final keys = <String>{};
        for (final v in _env) {
          if (!envKeyRe.hasMatch(v.key)) return 'Invalid variable name "${v.key}" (letters, digits and _).';
          if (!keys.add(v.key)) return 'The variable ${v.key} is set twice.';
        }
        if (_healthType == 'http' && !_healthPath.text.trim().startsWith('/')) return 'The health check path must start with /.';
        return null;
      case WizardStep.review:
        return null;
    }
  }

  /// Creates the app (first time) or renames it.
  Future<bool> _saveApp() async {
    final api = ref.read(apiProvider);
    final rt = _runtime!;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      if (_app == null) {
        final res = await api.post('/projects/$_projectId/apps', data: {
          'slug': _slug.text.trim(),
          'name': _name.text.trim(),
          'kind': _kind,
          'runtime': rt.id,
          if (!rt.takesImage) 'runtime_version': _version,
          'config': {
            if (rt.takesImage && !_fromGit) 'image': _image.text.trim(),
            if (_kind == 'web') 'port': int.parse(_port.text.trim()),
            if (_kind == 'cron') 'schedule': _schedule.text.trim(),
          },
          if (rt.takesImage && _fromGit) ...{
            'source': {
              'repo_url': _repo.text.trim(),
              'branch': _branch.text.trim(),
              'context_dir': _contextDir.text.trim(),
              'dockerfile': _dockerfile.text.trim(),
              if (_gitToken.text.trim().isNotEmpty) 'token': _gitToken.text.trim(),
            },
            'build': false, // the build starts at "Build & deploy"
          },
        });
        final app = App.fromJson(res is Map && res['app'] != null ? res['app'] : res);
        _app = app;
        _cpu = app.cpuMillicores > 0 ? app.cpuMillicores : _cpu;
        _memory = app.memoryMb > 0 ? app.memoryMb : _memory;
        _replicas = app.hasReplicas ? (app.replicas > 0 ? app.replicas : 1) : app.replicas;
        _healthType = app.healthType.isEmpty ? 'tcp' : app.healthType;
        _healthPath.text = app.healthPath.isEmpty ? '/' : app.healthPath;
        ref.invalidate(projectAppsProvider(_projectId!));
        ref.invalidate(projectsProvider);
      } else {
        _app = App.fromJson(await api.patch('/apps/${_app!.id}', data: {'name': _name.text.trim()}));
      }
      return true;
    } on Object catch (e) {
      setState(() => _error = describeError(e));
      return false;
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _pickAndUpload() async {
    final rt = _runtime!;
    PickedArtifact? file;
    try {
      file = await pickArtifact(rt);
    } on Object catch (e) {
      setState(() => _uploadError = 'Could not open the file: ${describeError(e)}');
      return;
    }
    if (file == null) return;
    final problem = artifactProblem(rt, file.name);
    if (problem != null) {
      setState(() => _uploadError = problem);
      return;
    }
    _file = file;
    await _upload();
  }

  Future<void> _upload() async {
    final file = _file;
    final app = _app;
    if (file == null || app == null) return;
    final cancel = CancelToken();
    setState(() {
      _cancel = cancel;
      _artifact = null;
      _uploadError = null;
      _uploadProgress = 0;
      _error = null;
    });
    try {
      final art = await uploadArtifact(
        ref.read(apiProvider),
        app.id,
        file,
        cancel: cancel,
        onProgress: (p) {
          if (mounted) setState(() => _uploadProgress = p);
        },
      );
      if (!mounted) return;
      setState(() {
        _artifact = art;
        _uploadProgress = null;
        if (_kind == 'web' && art.detect.port > 0 && _port.text.trim() == '${_runtime!.port}') {
          // keep the user's port unless it is still the runtime default
          _port.text = '${art.detect.port}';
        }
        if (art.detect.healthPath.isNotEmpty && _healthType == 'tcp') {
          _healthType = 'http';
          _healthPath.text = art.detect.healthPath;
        }
      });
    } on Object catch (e) {
      if (mounted) {
        setState(() {
          _uploadProgress = null;
          _uploadError = describeError(e);
        });
      }
    } finally {
      _cancel = null;
    }
  }

  Future<void> _deploy({bool force = false}) async {
    final api = ref.read(apiProvider);
    final app = _app!;
    final rt = _runtime!;
    setState(() {
      _busy = true;
      _error = null;
      _forceOffered = false;
    });
    try {
      await api.patch('/apps/${app.id}', data: {
        if (!rt.takesImage && _version != null) 'runtime_version': _version,
        'config': {
          if (rt.takesImage && !_fromGit) 'image': _image.text.trim(),
          if (_kind == 'web') 'port': int.parse(_port.text.trim()),
          if (_kind == 'cron') 'schedule': _schedule.text.trim(),
          if (app.hasReplicas) 'replicas': _replicas,
          'resources': {'cpu_millicores': _cpu, 'memory_mb': _memory},
          if (_kind == 'web') 'health': {'type': _healthType, if (_healthType == 'http') 'path': _healthPath.text.trim()},
          'args': _args.text.trim(),
        },
      });
      if (_env.isNotEmpty) {
        await api.put('/apps/${app.id}/env', data: [for (final v in _env) v.toJson()]);
      }
      if (_fromGit) {
        // the build deploys when it is done: follow it on the app's Builds tab
        await api.post('/apps/${app.id}/builds', data: {'deploy': true});
        ref.invalidate(projectAppsProvider(app.projectId));
        if (mounted) {
          showSnack(context, 'Build started: it deploys ${app.name} when it is done');
          context.go('/apps/${app.id}');
        }
        return;
      }
      final res = await api.post('/apps/${app.id}/deploy', data: {
        if (_artifact != null) 'artifact_id': _artifact!.id,
        if (force) 'force': true,
      });
      setState(() => _release = Release.fromJson(res));
    } on ApiException catch (e) {
      setState(() {
        _error = e.message;
        // capacity / quota: the API refuses with 409 unless forced
        _forceOffered = e.status == 409 && !e.message.contains('already in progress');
      });
    } on Object catch (e) {
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _close() async {
    final app = _app;
    if (app != null && !_deploying) {
      final discard = await showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: const Text('Leave the wizard?'),
          content: Text('"${app.name}" was created but not deployed yet. Keep it to finish later, or discard it.'),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Keep')),
            FilledButton(
              style: FilledButton.styleFrom(backgroundColor: NectarColors.of(ctx).failed),
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Discard'),
            ),
          ],
        ),
      );
      if (discard == null) return;
      if (discard) {
        try {
          await ref.read(apiProvider).delete('/apps/${app.id}');
          ref.invalidate(projectAppsProvider(app.projectId));
          ref.invalidate(projectsProvider);
        } on Object catch (e) {
          if (mounted) showSnack(context, 'Could not discard: ${describeError(e)}', error: true);
          return;
        }
      }
    }
    if (!mounted) return;
    if (_app != null && _deploying) {
      context.go('/apps/${_app!.id}');
    } else if (_projectId != null) {
      context.go('/projects/$_projectId');
    } else {
      context.go('/projects');
    }
  }

  // ------------------------------------------------------------------ layout --

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.sizeOf(context).width >= 980;
    final steps = _steps;
    final index = steps.indexOf(_step);
    final theme = Theme.of(context);
    final c = NectarColors.of(context);

    final content = _deploying ? _deployView() : _stepView(_step);
    final footer = _deploying
        ? null
        : Row(
            children: [
              if (index > 0) OutlinedButton(onPressed: _busy ? null : _back, child: const Text('Back')),
              const Spacer(),
              if (_step != WizardStep.review)
                SizedBox(
                  width: 160,
                  child: BusyButton(
                    label: _step == WizardStep.name && _app == null ? 'Create & continue' : 'Continue',
                    busy: _busy,
                    onPressed: _next,
                  ),
                )
              else ...[
                if (_forceOffered)
                  Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: OutlinedButton(onPressed: _busy ? null : () => _deploy(force: true), child: const Text('Deploy anyway')),
                  ),
                SizedBox(width: 160, child: BusyButton(label: _fromGit ? 'Build & deploy' : 'Deploy', busy: _busy, onPressed: _deploy)),
              ],
            ],
          );

    final card = Card(
      child: Padding(
        padding: EdgeInsets.all(wide ? 28 : 18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (!wide && !_deploying) ...[
              Text('Step ${index + 1} of ${steps.length}', style: theme.textTheme.labelSmall),
              const SizedBox(height: 6),
              ClipRRect(
                borderRadius: BorderRadius.circular(99),
                child: LinearProgressIndicator(value: (index + 1) / steps.length, minHeight: 4),
              ),
              const SizedBox(height: 18),
            ],
            Text(_deploying ? 'Deploying ${_app!.name}' : _step.title, style: theme.textTheme.headlineSmall),
            const SizedBox(height: 18),
            if (_error != null) FormError(_error!),
            content,
            if (footer != null) ...[const SizedBox(height: 26), footer],
          ],
        ),
      ),
    );

    return PageScaffold(
      title: 'New app',
      subtitle: _app == null ? null : '${_app!.name} · ${runtimeInfo(_app!.runtime).label}',
      maxWidth: 1080,
      actions: [IconButton(tooltip: 'Close', icon: const Icon(Icons.close_rounded), onPressed: _busy ? null : _close)],
      children: [
        if (wide)
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              SizedBox(
                width: 220,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    for (var i = 0; i < steps.length; i++)
                      _StepItem(
                        number: i + 1,
                        title: steps[i].title,
                        state: _deploying || i < index ? 1 : (i == index ? 0 : -1),
                        onTap: !_deploying && i < index && !_busy ? () => _go(steps[i]) : null,
                      ),
                    if (_deploying)
                      _StepItem(number: steps.length + 1, title: 'Deploy', state: _deployOk == true ? 1 : 0, onTap: null),
                  ],
                ),
              ),
              const SizedBox(width: NectarTokens.gapLarge),
              Expanded(child: card),
            ],
          )
        else
          card,
        if (!_deploying && _app != null)
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: Text(
              '${_app!.name} is saved as "not deployed"; you can close the wizard and continue later from the app page.',
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
          ),
      ],
    );
  }

  Widget _stepView(WizardStep s) => switch (s) {
        WizardStep.project => _projectStep(),
        WizardStep.type => _typeStep(),
        WizardStep.version => _versionStep(),
        WizardStep.name => _nameStep(),
        WizardStep.artifact => _artifactStep(),
        WizardStep.resources => _resourcesStep(),
        WizardStep.settings => _settingsStep(),
        WizardStep.review => _reviewStep(),
      };

  // ------------------------------------------------------------------- steps --

  Widget _projectStep() {
    final projects = ref.watch(projectsProvider);
    final canCreate = ref.watch(sessionProvider.select((s) => s.can('project.create')));
    return AsyncBlock(
      value: projects,
      onRetry: () => ref.invalidate(projectsProvider),
      data: (list) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (list.isEmpty) const Text('There are no projects yet. Create one first.'),
          for (final p in list)
            _ChoiceTile(
              selected: _projectId == p.id,
              title: p.name,
              subtitle: '${p.namespace} · ${p.apps} ${p.apps == 1 ? 'app' : 'apps'}',
              leading: const Icon(Icons.folder_rounded, color: NectarTokens.amber),
              onTap: _app != null ? null : () => setState(() => _projectId = p.id),
            ),
          if (canCreate && _app == null)
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                onPressed: () async {
                  final created = await showDialog<Project>(context: context, builder: (_) => const NewProjectDialog());
                  if (created != null) {
                    ref.invalidate(projectsProvider);
                    setState(() => _projectId = created.id);
                  }
                },
                icon: const Icon(Icons.add_rounded, size: 18),
                label: const Text('New project'),
              ),
            ),
        ],
      ),
    );
  }

  Widget _typeStep() {
    final runtimes = ref.watch(runtimesProvider);
    final locked = _app != null;
    return AsyncBlock(
      value: runtimes,
      onRetry: () => ref.invalidate(runtimesProvider),
      data: (list) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (locked)
            const Padding(
              padding: EdgeInsets.only(bottom: 12),
              child: Text('The type is fixed once the app exists. Discard the app to choose another type.'),
            ),
          Wrap(
            spacing: 12,
            runSpacing: 12,
            children: [
              for (final rt in list)
                _TypeCard(
                  runtime: rt,
                  selected: _runtime?.id == rt.id,
                  onTap: locked
                      ? null
                      : () => setState(() {
                            _runtime = rt;
                            _kind = rt.kinds.contains(_kind) ? _kind : (rt.kinds.isEmpty ? 'web' : rt.kinds.first);
                            _version = rt.defaultVersion?.id;
                            _port.text = '${rt.port > 0 ? rt.port : 8080}';
                          }),
                ),
            ],
          ),
          if (_runtime != null && _runtime!.kinds.length > 1) ...[
            const SizedBox(height: 22),
            const SectionLabel('Kind'),
            SegmentedButton<String>(
              segments: [
                for (final k in _runtime!.kinds)
                  ButtonSegment(value: k, label: Text(_kindLabel(k)), tooltip: _kindHelp(k)),
              ],
              selected: {_kind},
              showSelectedIcon: false,
              onSelectionChanged: locked ? null : (v) => setState(() => _kind = v.first),
            ),
            const SizedBox(height: 8),
            Text(_kindHelp(_kind), style: TextStyle(color: NectarColors.of(context).muted, fontSize: 12.5)),
          ],
        ],
      ),
    );
  }

  Widget _versionStep() {
    final rt = _runtime!;
    if (rt.takesImage) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_app == null) ...[
            Align(
              alignment: Alignment.centerLeft,
              child: SegmentedButton<bool>(
                segments: const [
                  ButtonSegment(value: false, icon: Icon(Icons.inventory_2_outlined, size: 18), label: Text('Existing image')),
                  ButtonSegment(value: true, icon: Icon(Icons.merge_type_rounded, size: 18), label: Text('Build from Git')),
                ],
                selected: {_fromGit},
                onSelectionChanged: (s) => setState(() => _fromGit = s.first),
              ),
            ),
            const SizedBox(height: 16),
          ],
          if (_fromGit)
            GitSourceFields(repo: _repo, branch: _branch, contextDir: _contextDir, dockerfile: _dockerfile, token: _gitToken)
          else ...[
          TextField(
            controller: _image,
            enabled: _app == null,
            style: monoStyle(context, size: 14),
            decoration: const InputDecoration(labelText: 'Image', hintText: 'ghcr.io/acme/shop:1.4.2'),
            autocorrect: false,
          ),
          const SizedBox(height: 8),
          Text(
            'Any public image, or a private one with a login under Settings → Registries. Pin a version tag (not :latest) '
            'so deploys and rollbacks are reproducible.',
            style: TextStyle(color: NectarColors.of(context).muted, fontSize: 12.5),
          ),
          ],
        ],
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final v in rt.versions)
          _ChoiceTile(
            selected: _version == v.id,
            title: v.label,
            subtitle: v.image,
            mono: true,
            trailing: v.isDefault ? const _Tag('recommended') : null,
            onTap: () => setState(() => _version = v.id),
          ),
      ],
    );
  }

  Widget _nameStep() {
    final settings = ref.watch(panelSettingsProvider);
    final projects = ref.watch(projectsProvider);
    final project = projects.hasValue ? projects.value!.where((p) => p.id == _projectId).firstOrNull : null;
    final c = NectarColors.of(context);
    final base = settings.hasValue ? settings.value!.baseDomain : '';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: _name,
          autofocus: _app == null,
          decoration: const InputDecoration(labelText: 'Name', hintText: 'Shop API'),
          textInputAction: TextInputAction.next,
        ),
        const SizedBox(height: 14),
        TextField(
          controller: _slug,
          enabled: _app == null,
          style: monoStyle(context, size: 14),
          decoration: const InputDecoration(labelText: 'Slug', helperText: 'Used in Kubernetes names and the default domain'),
          onChanged: (_) => _slugEdited = true,
        ),
        if (_kind == 'web') ...[
          const SizedBox(height: 14),
          TextField(
            controller: _port,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: const InputDecoration(labelText: 'Port your app listens on', helperText: 'Also available to the app as \$PORT'),
          ),
          const SizedBox(height: 18),
          ValueListenableBuilder(
            valueListenable: _slug,
            builder: (context, slug, _) {
              final host = base.isEmpty || project == null ? null : '${slug.text.isEmpty ? '…' : slug.text}-${project.slug}.$base';
              return Container(
                padding: const EdgeInsets.all(14),
                decoration: BoxDecoration(
                  color: c.elevated,
                  borderRadius: BorderRadius.circular(NectarTokens.radius),
                  border: Border.all(color: c.border),
                ),
                child: Row(
                  children: [
                    Icon(Icons.public_rounded, size: 20, color: host == null ? c.muted : NectarTokens.amber),
                    const SizedBox(width: 12),
                    Expanded(
                      child: host == null
                          ? Text(
                              'No base domain is set, so the app gets no domain automatically. '
                              'Set one in Settings, or add a domain on the app page later.',
                              style: TextStyle(color: c.muted, fontSize: 13),
                            )
                          : Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                SelectableText('https://$host', style: monoStyle(context, size: 13.5)),
                                Text('default domain, with a Let\'s Encrypt certificate', style: TextStyle(color: c.muted, fontSize: 12)),
                              ],
                            ),
                    ),
                  ],
                ),
              );
            },
          ),
        ],
        if (_kind == 'cron') ...[
          const SizedBox(height: 14),
          TextField(
            controller: _schedule,
            style: monoStyle(context, size: 14),
            decoration: const InputDecoration(labelText: 'Schedule (cron)', helperText: 'e.g. */15 * * * * = every 15 minutes, @daily'),
          ),
        ],
      ],
    );
  }

  Widget _artifactStep() {
    final rt = _runtime!;
    final c = NectarColors.of(context);
    final art = _artifact;
    final progress = _uploadProgress;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        InkWell(
          borderRadius: BorderRadius.circular(NectarTokens.radius),
          onTap: progress != null ? null : _pickAndUpload,
          child: Container(
            padding: const EdgeInsets.symmetric(vertical: 34, horizontal: 20),
            decoration: BoxDecoration(
              color: c.elevated,
              borderRadius: BorderRadius.circular(NectarTokens.radius),
              border: Border.all(color: art != null ? c.running.withValues(alpha: 0.5) : c.border, width: 1.5),
            ),
            child: Column(
              children: [
                HexBadge(
                  size: 52,
                  color: (art != null ? c.running : NectarTokens.amber).withValues(alpha: 0.16),
                  child: Icon(art != null ? Icons.check_rounded : Icons.upload_file_rounded, color: art != null ? c.running : NectarTokens.amber),
                ),
                const SizedBox(height: 12),
                Text(
                  art != null ? art.filename : (_file?.name ?? 'Choose your ${rt.accept.join(' / ')} file'),
                  style: Theme.of(context).textTheme.titleMedium,
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 4),
                Text(
                  art != null
                      ? '${formatBytes(art.size)} · sha256 ${art.sha256.substring(0, 12)}… · click to replace'
                      : (progress != null ? 'Uploading… ${(progress * 100).round()}%' : _artifactHint(rt)),
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                  textAlign: TextAlign.center,
                ),
                if (progress != null) ...[
                  const SizedBox(height: 14),
                  ClipRRect(
                    borderRadius: BorderRadius.circular(99),
                    child: LinearProgressIndicator(value: progress, minHeight: 6),
                  ),
                  TextButton(onPressed: () => _cancel?.cancel(), child: const Text('Cancel upload')),
                ],
              ],
            ),
          ),
        ),
        if (_uploadError != null) ...[
          const SizedBox(height: 12),
          FormError(_uploadError!),
          if (_file != null && _file!.canRetry)
            Align(alignment: Alignment.centerLeft, child: TextButton(onPressed: _upload, child: const Text('Retry upload'))),
        ],
        if (art != null) ...[const SizedBox(height: 16), _detectView(art.detect)],
      ],
    );
  }

  Widget _detectView(DetectResult d) {
    final rt = _runtime!;
    final c = NectarColors.of(context);
    final facts = <String>[
      if (d.minJava > 0) 'Needs Java ${d.minJava} or newer',
      if (d.springBoot) 'Spring Boot ${d.springBootVersion}'.trim(),
      if (d.elfArch.isNotEmpty) 'Linux ${d.elfArch} binary (${d.elfLibc})',
      if (d.archiveMarkers.isNotEmpty) 'Contains ${d.archiveMarkers.join(', ')}',
      if (d.port > 0) 'Listens on port ${d.port}',
    ];
    final suggested = d.runtime == rt.id && d.version.isNotEmpty && d.version != _version
        ? rt.versions.where((v) => v.id == d.version).firstOrNull
        : null;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final f in facts)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 3),
            child: Row(children: [
              Icon(Icons.check_rounded, size: 16, color: c.running),
              const SizedBox(width: 8),
              Expanded(child: Text(f, style: const TextStyle(fontSize: 13.5))),
            ]),
          ),
        if (suggested != null)
          Container(
            margin: const EdgeInsets.only(top: 10),
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: NectarTokens.amber.withValues(alpha: 0.08),
              borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
              border: Border.all(color: NectarTokens.amber.withValues(alpha: 0.3)),
            ),
            child: Row(
              children: [
                const Icon(Icons.lightbulb_outline_rounded, size: 18, color: NectarTokens.amber),
                const SizedBox(width: 10),
                Expanded(child: Text('This file suits ${suggested.label} better than the version you chose.')),
                TextButton(onPressed: () => setState(() => _version = suggested.id), child: Text('Use ${suggested.label}')),
              ],
            ),
          ),
        for (final w in d.warnings)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Icon(Icons.warning_amber_rounded, size: 16, color: c.warning),
              const SizedBox(width: 8),
              Expanded(child: Text(w, style: const TextStyle(fontSize: 13.5))),
            ]),
          ),
      ],
    );
  }

  Widget _resourcesStep() {
    final c = NectarColors.of(context);
    final preset = _customResources ? null : presetFor(_cpu, _memory);
    final app = _app!;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionLabel('Per replica'),
        Wrap(
          spacing: 10,
          runSpacing: 10,
          children: [
            for (final p in resourcePresets)
              _PresetCard(
                title: p.label,
                subtitle: '${formatCpu(p.cpuMillicores)} · ${formatMb(p.memoryMb)}',
                selected: preset?.id == p.id,
                onTap: () => setState(() {
                  _customResources = false;
                  _cpu = p.cpuMillicores;
                  _memory = p.memoryMb;
                }),
              ),
            _PresetCard(
              title: 'Custom',
              subtitle: preset == null ? '${formatCpu(_cpu)} · ${formatMb(_memory)}' : 'your values',
              selected: preset == null,
              onTap: () => setState(() => _customResources = true),
            ),
          ],
        ),
        if (preset == null) ...[
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: _NumberField(
                  label: 'CPU (millicores)',
                  value: _cpu,
                  onChanged: (v) => setState(() => _cpu = v),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: _NumberField(
                  label: 'Memory (MB)',
                  value: _memory,
                  onChanged: (v) => setState(() => _memory = v),
                ),
              ),
            ],
          ),
        ],
        if (app.hasReplicas) ...[
          const SizedBox(height: 24),
          const SectionLabel('Replicas'),
          Row(
            children: [
              IconButton.outlined(
                onPressed: _replicas > 0 ? () => setState(() => _replicas--) : null,
                icon: const Icon(Icons.remove_rounded),
              ),
              SizedBox(
                width: 56,
                child: Text('$_replicas', textAlign: TextAlign.center, style: Theme.of(context).textTheme.headlineSmall),
              ),
              IconButton.outlined(
                onPressed: _replicas < 20 ? () => setState(() => _replicas++) : null,
                icon: const Icon(Icons.add_rounded),
              ),
              const SizedBox(width: 16),
              Expanded(
                child: Text(
                  _replicas == 0
                      ? 'Stopped: deployed but not running.'
                      : 'Total: ${formatCpu(_cpu * _replicas)} · ${formatMb(_memory * _replicas)}',
                  style: TextStyle(color: c.muted, fontSize: 13),
                ),
              ),
            ],
          ),
        ],
        if (_kind == 'cron') ...[
          const SizedBox(height: 24),
          TextField(
            controller: _schedule,
            style: monoStyle(context, size: 14),
            decoration: const InputDecoration(labelText: 'Schedule (cron)'),
          ),
        ],
      ],
    );
  }

  Widget _settingsStep() {
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionLabel('Environment variables'),
        EnvEditor(vars: _env, onChanged: () => setState(() {})),
        if (_kind == 'web') ...[
          const SizedBox(height: 24),
          const SectionLabel('Health check'),
          Wrap(
            spacing: 12,
            runSpacing: 12,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              SegmentedButton<String>(
                segments: const [
                  ButtonSegment(value: 'tcp', label: Text('Port open')),
                  ButtonSegment(value: 'http', label: Text('HTTP')),
                  ButtonSegment(value: 'none', label: Text('None')),
                ],
                selected: {_healthType},
                showSelectedIcon: false,
                onSelectionChanged: (v) => setState(() => _healthType = v.first),
              ),
              if (_healthType == 'http')
                SizedBox(
                  width: 260,
                  child: TextField(
                    controller: _healthPath,
                    style: monoStyle(context, size: 14),
                    decoration: const InputDecoration(labelText: 'Path', hintText: '/health'),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            'Traffic only reaches a replica once its health check passes, so a broken release never replaces a working one.',
            style: TextStyle(color: c.muted, fontSize: 12.5),
          ),
        ],
        const SizedBox(height: 24),
        const SectionLabel('Arguments'),
        TextField(
          controller: _args,
          style: monoStyle(context, size: 14),
          decoration: const InputDecoration(hintText: '--spring.profiles.active=prod', helperText: 'Passed to your app as \$APP_ARGS'),
        ),
      ],
    );
  }

  Widget _reviewStep() {
    final rt = _runtime!;
    final app = _app!;
    final version = rt.versions.where((v) => v.id == _version).firstOrNull;
    final rows = <(String, String)>[
      ('Project', ref.watch(projectsProvider).value?.where((p) => p.id == _projectId).firstOrNull?.name ?? ''),
      ('App', '${app.name} (${app.slug})'),
      ('Type', '${rt.name} · ${_kindLabel(_kind)}'),
      if (rt.takesImage && _fromGit) ('Source', '${_repo.text.trim()} (${_branch.text.trim()})')
      else if (rt.takesImage) ('Image', _image.text.trim())
      else ('Version', version?.label ?? _version ?? ''),
      if (_artifact != null) ('File', '${_artifact!.filename} (${formatBytes(_artifact!.size)})'),
      if (_kind == 'web') ('Port', _port.text.trim()),
      if (_kind == 'cron') ('Schedule', _schedule.text.trim()),
      ('Resources', '${formatCpu(_cpu)} · ${formatMb(_memory)} per replica'),
      if (app.hasReplicas) ('Replicas', '$_replicas'),
      if (_kind == 'web') ('Health check', _healthType == 'http' ? 'HTTP ${_healthPath.text.trim()}' : (_healthType == 'tcp' ? 'port open' : 'none')),
      ('Environment', _env.isEmpty ? 'none' : '${_env.length} variables (${_env.where((v) => v.secret).length} secret)'),
    ];
    final c = NectarColors.of(context);
    return Column(
      children: [
        for (final (k, v) in rows)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SizedBox(width: 140, child: Text(k, style: TextStyle(color: c.muted, fontSize: 13.5))),
                Expanded(child: SelectableText(v, style: const TextStyle(fontSize: 13.5))),
              ],
            ),
          ),
      ],
    );
  }

  Widget _deployView() {
    final app = _app!;
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        DeployProgress(
          app: app,
          releaseId: _release!.id,
          onFinished: (ok) {
            if (!mounted) return;
            setState(() => _deployOk = ok);
            ref.invalidate(projectAppsProvider(app.projectId));
          },
        ),
        const SizedBox(height: 22),
        if (_deployOk == null)
          Text('You can leave this page; the deploy continues on the server.', style: TextStyle(color: c.muted, fontSize: 12.5)),
        if (_deployOk != null)
          Wrap(
            spacing: 10,
            runSpacing: 10,
            children: [
              FilledButton(onPressed: () => context.go('/apps/${app.id}'), child: Text(_deployOk! ? 'Open ${app.name}' : 'Open the app to fix it')),
              OutlinedButton(onPressed: () => context.go('/projects/${app.projectId}'), child: const Text('Back to the project')),
            ],
          ),
      ],
    );
  }

  static String _artifactHint(Runtime rt) => switch (rt.artifactKind) {
        'archive' => 'A .zip or .tar.gz of your app; a single top-level folder (dist/, build/) is unwrapped.',
        _ => rt.accept.contains('*') ? 'Your compiled executable for Linux (amd64 or arm64).' : 'The file you run in production.',
      };
}

String _kindLabel(String k) => switch (k) {
      'web' => 'Web',
      'worker' => 'Worker',
      'cron' => 'Scheduled',
      'task' => 'Task',
      _ => k,
    };

String _kindHelp(String k) => switch (k) {
      'web' => 'Serves HTTP on a port and gets a domain.',
      'worker' => 'Runs continuously in the background, no domain.',
      'cron' => 'Runs on a schedule and exits.',
      'task' => 'Runs once when you start it (migrations, scripts).',
      _ => '',
    };

// ------------------------------------------------------------------ pieces --

class _StepItem extends StatelessWidget {
  const _StepItem({required this.number, required this.title, required this.state, required this.onTap});

  final int number;
  final String title;

  /// 1 done, 0 current, -1 upcoming
  final int state;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final current = state == 0;
    return InkWell(
      borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 9, horizontal: 6),
        child: Row(
          children: [
            HexBadge(
              size: 26,
              color: state == 1 ? c.running.withValues(alpha: 0.2) : (current ? NectarTokens.amber : c.elevated),
              child: state == 1
                  ? Icon(Icons.check_rounded, size: 13, color: c.running)
                  : Text(
                      '$number',
                      style: TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: current ? NectarTokens.onAmber : c.muted),
                    ),
            ),
            const SizedBox(width: 10),
            Text(
              title,
              style: TextStyle(fontSize: 13.5, fontWeight: current ? FontWeight.w600 : FontWeight.w500, color: state < 0 ? c.muted : c.text),
            ),
          ],
        ),
      ),
    );
  }
}

class _ChoiceTile extends StatelessWidget {
  const _ChoiceTile({
    required this.selected,
    required this.title,
    this.subtitle,
    this.leading,
    this.trailing,
    this.mono = false,
    required this.onTap,
  });

  final bool selected;
  final String title;
  final String? subtitle;
  final Widget? leading;
  final Widget? trailing;
  final bool mono;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: InkWell(
        borderRadius: BorderRadius.circular(NectarTokens.radius),
        onTap: onTap,
        child: AnimatedContainer(
          duration: NectarTokens.fast,
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          decoration: BoxDecoration(
            color: selected ? NectarTokens.amber.withValues(alpha: 0.08) : c.surface,
            borderRadius: BorderRadius.circular(NectarTokens.radius),
            border: Border.all(color: selected ? NectarTokens.amber : c.border, width: selected ? 1.5 : 1),
          ),
          child: Row(
            children: [
              if (leading != null) ...[leading!, const SizedBox(width: 12)],
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(title, style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w600)),
                    if (subtitle != null)
                      Text(subtitle!, style: mono ? monoStyle(context, size: 12, color: c.muted) : TextStyle(fontSize: 12.5, color: c.muted)),
                  ],
                ),
              ),
              if (trailing != null) trailing!,
              const SizedBox(width: 8),
              Icon(
                selected ? Icons.radio_button_checked_rounded : Icons.radio_button_unchecked_rounded,
                size: 20,
                color: selected ? NectarTokens.amber : c.border,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _TypeCard extends StatelessWidget {
  const _TypeCard({required this.runtime, required this.selected, required this.onTap});

  final Runtime runtime;
  final bool selected;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final info = runtimeInfo(runtime.id);
    return SizedBox(
      width: 220,
      child: InkWell(
        borderRadius: BorderRadius.circular(NectarTokens.radius),
        onTap: onTap,
        child: AnimatedContainer(
          duration: NectarTokens.fast,
          height: 150,
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            color: selected ? NectarTokens.amber.withValues(alpha: 0.08) : c.surface,
            borderRadius: BorderRadius.circular(NectarTokens.radius),
            border: Border.all(color: selected ? NectarTokens.amber : c.border, width: selected ? 1.5 : 1),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              HexBadge(
                size: 42,
                color: NectarTokens.amber.withValues(alpha: selected ? 0.25 : 0.12),
                child: Icon(info.icon, size: 19, color: NectarTokens.amber),
              ),
              const SizedBox(height: 12),
              Text(runtime.name, style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w600)),
              const SizedBox(height: 4),
              Expanded(
                child: Text(
                  runtime.description,
                  maxLines: 3,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(fontSize: 12.5, color: c.muted, height: 1.35),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _PresetCard extends StatelessWidget {
  const _PresetCard({required this.title, required this.subtitle, required this.selected, required this.onTap});

  final String title;
  final String subtitle;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return InkWell(
      borderRadius: BorderRadius.circular(NectarTokens.radius),
      onTap: onTap,
      child: Container(
        width: 150,
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: selected ? NectarTokens.amber.withValues(alpha: 0.08) : c.surface,
          borderRadius: BorderRadius.circular(NectarTokens.radius),
          border: Border.all(color: selected ? NectarTokens.amber : c.border, width: selected ? 1.5 : 1),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            Text(subtitle, style: TextStyle(fontSize: 12.5, color: c.muted)),
          ],
        ),
      ),
    );
  }
}

class _NumberField extends StatefulWidget {
  const _NumberField({required this.label, required this.value, required this.onChanged});

  final String label;
  final int value;
  final ValueChanged<int> onChanged;

  @override
  State<_NumberField> createState() => _NumberFieldState();
}

class _NumberFieldState extends State<_NumberField> {
  late final _c = TextEditingController(text: '${widget.value}');

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => TextField(
        controller: _c,
        keyboardType: TextInputType.number,
        inputFormatters: [FilteringTextInputFormatter.digitsOnly],
        decoration: InputDecoration(labelText: widget.label),
        onChanged: (v) {
          final n = int.tryParse(v);
          if (n != null) widget.onChanged(n);
        },
      );
}

class _Tag extends StatelessWidget {
  const _Tag(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
        decoration: BoxDecoration(
          color: NectarTokens.amber.withValues(alpha: 0.14),
          borderRadius: BorderRadius.circular(99),
        ),
        child: Text(text, style: const TextStyle(fontSize: 11.5, fontWeight: FontWeight.w600, color: NectarTokens.amber)),
      );
}
