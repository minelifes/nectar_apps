import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Browse, upload, download, edit, rename and delete the files of an app volume.
Future<void> showVolumeFiles(
  BuildContext context, {
  required int appId,
  required String volume,
}) => showDialog<void>(
  context: context,
  builder: (_) => _VolumeFilesDialog(appId: appId, volume: volume),
);

const _maxEdit = 1024 * 1024; // files up to 1 MB open in the editor

class _VolumeFilesDialog extends ConsumerStatefulWidget {
  const _VolumeFilesDialog({required this.appId, required this.volume});

  final int appId;
  final String volume;

  @override
  ConsumerState<_VolumeFilesDialog> createState() => _VolumeFilesDialogState();
}

class _VolumeFilesDialogState extends ConsumerState<_VolumeFilesDialog> {
  String _path = '/';
  List<VolumeEntry>? _entries;
  String? _error;
  bool _busy = false;
  bool _starting = true;
  late final Api _api;

  String get _base => '/apps/${widget.appId}/volumes/${widget.volume}/files';

  @override
  void initState() {
    super.initState();
    _api = ref.read(apiProvider);
    _load();
  }

  String _join(String name) => _path == '/' ? '/$name' : '$_path/$name';

  Future<void> _load([String? path]) async {
    final target = path ?? _path;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final res = await ref
          .read(apiProvider)
          .get(_base, query: {'path': target});
      if (!mounted) return;
      setState(() {
        _path = (res as Map)['path'] as String? ?? target;
        _entries = listOf(res['entries'], VolumeEntry.fromJson);
      });
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) {
        setState(() {
          _busy = false;
          _starting = false;
        });
      }
    }
  }

  Future<void> _act(Map<String, dynamic> body, String done) async {
    setState(() => _busy = true);
    try {
      await ref.read(apiProvider).post(_base, data: body);
      if (mounted) showSnack(context, done);
      await _load();
    } on Object catch (e) {
      if (mounted) {
        showSnack(context, describeError(e), error: true);
        setState(() => _busy = false);
      }
    }
  }

  Future<void> _upload() async {
    final s = S.of(context);
    final res = await FilePicker.platform.pickFiles(withData: true);
    final f = res?.files.single;
    if (f == null || f.bytes == null) return;
    if (f.size > 64 * 1024 * 1024) {
      if (mounted) showSnack(context, s.appFilesUpTo64Mb, error: true);
      return;
    }
    final exists = _entries?.any((e) => e.name == f.name) ?? false;
    if (exists &&
        mounted &&
        !await confirmAction(
          context,
          s.appReplace(f.name),
          s.appTheFileInThisFolder,
          s.appReplace2,
        )) {
      return;
    }
    await _write(_join(f.name), f.bytes!, 'Uploaded ${f.name}');
  }

  Future<void> _write(String path, Uint8List bytes, String done) async {
    setState(() => _busy = true);
    try {
      await ref
          .read(apiProvider)
          .upload(
            '$_base/content',
            data: Stream.value(bytes),
            length: bytes.length,
            query: {'path': path},
          );
      if (mounted) showSnack(context, done);
      await _load();
    } on Object catch (e) {
      if (mounted) {
        showSnack(context, describeError(e), error: true);
        setState(() => _busy = false);
      }
    }
  }

  Future<Uint8List?> _read(VolumeEntry e) async {
    final res = await ref
        .read(apiProvider)
        .get(
          '$_base/content',
          query: {'path': _join(e.name)},
          options: Options(responseType: ResponseType.bytes),
        );
    return Uint8List.fromList((res as List).cast<int>());
  }

  Future<void> _download(VolumeEntry e) async {
    final s = S.of(context);
    final api = ref.read(apiProvider);
    if (!api.useTokens) {
      // the web build served by the panel: a plain download with the session cookie
      final q = Uri(queryParameters: {'path': _join(e.name)}).query;
      await launchUrl(Uri.parse('${api.baseUrl}/api$_base/content?$q'));
      return;
    }
    setState(() => _busy = true);
    try {
      final bytes = await _read(e);
      if (bytes == null) return;
      final saved = await FilePicker.platform.saveFile(
        fileName: e.name,
        bytes: bytes,
      );
      if (mounted && saved != null) showSnack(context, s.appSaved(e.name));
    } on Object catch (err) {
      if (mounted) showSnack(context, describeError(err), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _edit(VolumeEntry e) async {
    final s = S.of(context);
    setState(() => _busy = true);
    String? text;
    try {
      final bytes = await _read(e);
      text = utf8.decode(bytes ?? Uint8List(0), allowMalformed: false);
    } on FormatException {
      if (mounted) showSnack(context, s.appIsNotATextFile(e.name), error: true);
    } on Object catch (err) {
      if (mounted) showSnack(context, describeError(err), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
    if (text == null || !mounted) return;
    final edited = await showDialog<String>(
      context: context,
      builder: (_) => _EditFileDialog(name: e.name, text: text!),
    );
    if (edited != null && edited != text) {
      await _write(
        _join(e.name),
        Uint8List.fromList(utf8.encode(edited)),
        'Saved ${e.name}',
      );
    }
  }

  Future<void> _newFolder() async {
    final name = await promptText(context, 'New folder', 'Name');
    if (name == null || name.trim().isEmpty) return;
    await _act({
      'action': 'mkdir',
      'path': _join(name.trim()),
    }, 'Folder ${name.trim()} created');
  }

  Future<void> _rename(VolumeEntry e) async {
    final name = await promptText(
      context,
      'Rename ${e.name}',
      'New name',
      initial: e.name,
    );
    if (name == null || name.trim().isEmpty || name.trim() == e.name) return;
    await _act({
      'action': 'rename',
      'path': _join(e.name),
      'to': _join(name.trim()),
    }, 'Renamed to ${name.trim()}');
  }

  Future<void> _delete(VolumeEntry e) async {
    final s = S.of(context);
    final what = e.isDir ? 'the folder ${e.name} and everything in it' : e.name;
    if (!await confirmAction(
      context,
      s.appDelete2(what),
      s.appThisCannotBeUndoneBackups,
      s.actionDelete,
      danger: true,
    )) {
      return;
    }
    await _act({
      'action': 'delete',
      'path': _join(e.name),
    }, 'Deleted ${e.name}');
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final parts = _path.split('/').where((p) => p.isNotEmpty).toList();
    return AlertDialog(
      title: Row(
        children: [
          Expanded(child: Text(s.appFiles2(widget.volume))),
          if (_busy)
            const SizedBox(
              width: 16,
              height: 16,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
        ],
      ),
      content: SizedBox(
        width: 720,
        height: 480,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Wrap(
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                TextButton(
                  onPressed: _busy ? null : () => _load('/'),
                  child: Text(
                    widget.volume,
                    style: monoStyle(context, size: 13),
                  ),
                ),
                for (var i = 0; i < parts.length; i++) ...[
                  Text('/', style: TextStyle(color: c.muted)),
                  TextButton(
                    onPressed: _busy
                        ? null
                        : () => _load('/${parts.sublist(0, i + 1).join('/')}'),
                    child: Text(parts[i], style: monoStyle(context, size: 13)),
                  ),
                ],
              ],
            ),
            Row(
              children: [
                OutlinedButton.icon(
                  onPressed: _busy || _entries == null ? null : _upload,
                  icon: const Icon(Icons.upload_rounded, size: 18),
                  label: Text(s.appUpload),
                ),
                const SizedBox(width: 8),
                OutlinedButton.icon(
                  onPressed: _busy || _entries == null ? null : _newFolder,
                  icon: const Icon(Icons.create_new_folder_outlined, size: 18),
                  label: Text(s.appNewFolder),
                ),
                const Spacer(),
                IconButton(
                  tooltip: s.actionRefresh,
                  onPressed: _busy ? null : () => _load(),
                  icon: const Icon(Icons.refresh_rounded, size: 18),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Expanded(child: _body(context, c)),
            const SizedBox(height: 6),
            Text(
              s.appChangesApplyToTheRunning,
              style: TextStyle(color: c.muted, fontSize: 12),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionClose),
        ),
      ],
    );
  }

  Widget _body(BuildContext context, NectarColors c) {
    final s = S.of(context);
    if (_starting) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const CircularProgressIndicator(),
            const SizedBox(height: 12),
            Text(s.appOpeningTheVolume, style: TextStyle(color: c.muted)),
          ],
        ),
      );
    }
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              _error!,
              textAlign: TextAlign.center,
              style: TextStyle(color: c.failed),
            ),
            const SizedBox(height: 8),
            TextButton(onPressed: () => _load(), child: Text(s.actionRetry)),
          ],
        ),
      );
    }
    final list = _entries ?? const <VolumeEntry>[];
    if (list.isEmpty) {
      return Center(
        child: Text(s.appEmptyFolder, style: TextStyle(color: c.muted)),
      );
    }
    return ListView.separated(
      itemCount: list.length + (_path == '/' ? 0 : 1),
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, i) {
        if (_path != '/' && i == 0) {
          final up = _path.substring(0, _path.lastIndexOf('/'));
          return ListTile(
            dense: true,
            leading: const Icon(Icons.arrow_upward_rounded, size: 18),
            title: const Text('..'),
            onTap: _busy ? null : () => _load(up.isEmpty ? '/' : up),
          );
        }
        final e = list[i - (_path == '/' ? 0 : 1)];
        return ListTile(
          dense: true,
          leading: Icon(
            e.isDir
                ? Icons.folder_rounded
                : (e.type == 'link'
                      ? Icons.link_rounded
                      : Icons.insert_drive_file_outlined),
            size: 18,
            color: e.isDir ? NectarTokens.amber : c.muted,
          ),
          title: Text(e.name, style: monoStyle(context, size: 13)),
          subtitle: Text(
            [
              if (!e.isDir) formatBytes(e.size),
              if (e.modified != null) timeAgo(e.modified),
              e.mode,
              'owner ${e.owner}',
            ].join(' · '),
            style: TextStyle(color: c.muted, fontSize: 11.5),
          ),
          onTap: _busy
              ? null
              : (e.isDir
                    ? () => _load(_join(e.name))
                    : (e.type == 'file' && e.size <= _maxEdit
                          ? () => _edit(e)
                          : null)),
          trailing: PopupMenuButton<String>(
            enabled: !_busy,
            tooltip: s.domainActions,
            onSelected: (v) => switch (v) {
              'edit' => _edit(e),
              'download' => _download(e),
              'rename' => _rename(e),
              'delete' => _delete(e),
              _ => null,
            },
            itemBuilder: (_) => [
              if (e.type == 'file' && e.size <= _maxEdit)
                PopupMenuItem(value: 'edit', child: Text(s.backupEdit)),
              if (e.type == 'file')
                PopupMenuItem(value: 'download', child: Text(s.appDownload)),
              PopupMenuItem(value: 'rename', child: Text(s.appRenameMove)),
              PopupMenuItem(value: 'delete', child: Text(s.actionDelete)),
            ],
          ),
        );
      },
    );
  }

  @override
  void dispose() {
    // stop the helper pod right away (it would stop by itself after an hour)
    _api.post('$_base/close').ignore();
    super.dispose();
  }
}

class _EditFileDialog extends StatefulWidget {
  const _EditFileDialog({required this.name, required this.text});

  final String name;
  final String text;

  @override
  State<_EditFileDialog> createState() => _EditFileDialogState();
}

class _EditFileDialogState extends State<_EditFileDialog> {
  late final _ctl = TextEditingController(text: widget.text);

  @override
  void dispose() {
    _ctl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return AlertDialog(
      title: Text(widget.name),
      content: SizedBox(
        width: 760,
        height: 480,
        child: TextField(
          controller: _ctl,
          expands: true,
          maxLines: null,
          minLines: null,
          textAlignVertical: TextAlignVertical.top,
          style: monoStyle(context, size: 13),
          decoration: const InputDecoration(border: OutlineInputBorder()),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context, _ctl.text),
          child: Text(s.actionSave),
        ),
      ],
    );
  }
}
