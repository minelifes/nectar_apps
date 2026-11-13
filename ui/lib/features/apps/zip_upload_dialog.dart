import 'dart:async';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:file_picker/file_picker.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:nectar_apps_ui/api/session.dart';
import 'package:nectar_apps_ui/theme/tokens.dart';

import '../../api/client.dart';
import '../../core/format.dart';
import '../../generated/l10n.dart';

/// Uploads a picked .zip in chunks (any size up to the server's limit; the file is streamed, never held
/// in memory whole), then has the server unpack it into [path] of the volume and shows the progress.
/// Returns the final status ({files, dirs, skipped, …}) or null when cancelled or failed.
Future<Map<String, dynamic>?> showZipUploadDialog(
  BuildContext context, {
  required String base, // /apps/<id>/volumes/<name>
  required String path,
  required PlatformFile file,
}) => showDialog<Map<String, dynamic>>(
  context: context,
  barrierDismissible: false,
  builder: (_) => _ZipUploadDialog(base: base, path: path, file: file),
);

class _ZipUploadDialog extends ConsumerStatefulWidget {
  const _ZipUploadDialog({
    required this.base,
    required this.path,
    required this.file,
  });

  final String base;
  final String path;
  final PlatformFile file;

  @override
  ConsumerState<_ZipUploadDialog> createState() => _ZipUploadDialogState();
}

class _ZipUploadDialogState extends ConsumerState<_ZipUploadDialog> {
  String _state =
      'starting'; // starting | uploading | checking | unpacking | done | error
  int _sent = 0;
  double _percent = 0;
  String? _error;
  String? _id;
  bool _cancelled = false;
  final _cancelToken = CancelToken();

  String get _uploads => '${widget.base}/zip-uploads';

  @override
  void initState() {
    super.initState();
    unawaited(_run());
  }

  void _set(VoidCallback fn) {
    if (mounted && !_cancelled) setState(fn);
  }

  Future<void> _run() async {
    final api = ref.read(apiProvider);
    try {
      final created = await api.post(
        _uploads,
        data: {'size': widget.file.size, 'filename': widget.file.name},
      );
      _id = (created as Map)['id'] as String;
      final chunkSize = (created['chunk_size'] as num?)?.toInt() ?? 8 << 20;
      _set(() => _state = 'uploading');

      final stream = widget.file.readStream;
      if (stream == null) throw StateError('the file could not be read');
      var offset = 0;
      final buf = BytesBuilder(copy: false);
      Future<void> flush(Uint8List bytes) async {
        offset = await _sendChunk(api, bytes, offset);
        _set(() {
          _sent = offset;
          _percent = widget.file.size == 0 ? 0 : offset / widget.file.size;
        });
      }

      // read the file only as fast as the chunks go out
      final it = StreamIterator(stream);
      while (await it.moveNext()) {
        if (_cancelled) return;
        buf.add(it.current);
        if (buf.length >= chunkSize) {
          final all = buf.takeBytes();
          var i = 0;
          for (; all.length - i >= chunkSize; i += chunkSize) {
            await flush(Uint8List.sublistView(all, i, i + chunkSize));
            if (_cancelled) return;
          }
          if (i < all.length) buf.add(Uint8List.sublistView(all, i));
        }
      }
      if (buf.isNotEmpty) await flush(buf.takeBytes());
      if (_cancelled) return;
      if (offset != widget.file.size) {
        throw StateError('the file changed while it was uploaded');
      }

      await api.post('$_uploads/$_id/unzip', data: {'path': widget.path});
      _set(() {
        _state = 'checking';
        _percent = 0;
      });
      while (!_cancelled) {
        await Future<void>.delayed(const Duration(seconds: 1));
        final st = await api.get('$_uploads/$_id') as Map;
        final state = st['state'] as String? ?? '';
        if (state == 'done') {
          if (mounted) Navigator.pop(context, Map<String, dynamic>.from(st));
          return;
        }
        if (state == 'error') {
          _set(() {
            _state = 'error';
            _error = st['message'] as String? ?? 'unpacking failed';
          });
          return;
        }
        _set(() {
          _state = state;
          _percent = ((st['percent'] as num?)?.toDouble() ?? 0) / 100;
        });
      }
    } on Object catch (e) {
      if (_cancelled) return;
      _set(() {
        _state = 'error';
        _error = describeError(e);
      });
      _discard();
    }
  }

  /// Sends one chunk at [offset]; a failed chunk is sent again (up to 3 times) unless the server
  /// already has it. Returns the new offset.
  Future<int> _sendChunk(Api api, Uint8List bytes, int offset) async {
    for (var attempt = 1; ; attempt++) {
      try {
        final res = await api.upload(
          '$_uploads/$_id',
          data: Stream.value(bytes),
          length: bytes.length,
          query: {'offset': offset},
          cancel: _cancelToken,
        );
        return ((res as Map)['offset'] as num).toInt();
      } on Object {
        if (_cancelled || attempt >= 3) rethrow;
        await Future<void>.delayed(Duration(seconds: attempt * 2));
        try {
          final st = await api.get('$_uploads/$_id') as Map;
          final have = (st['offset'] as num?)?.toInt() ?? offset;
          if (have == offset + bytes.length) return have; // it did arrive
        } on Object {
          // try again
        }
      }
    }
  }

  void _discard() {
    final id = _id;
    if (id == null) return;
    unawaited(
      ref
          .read(apiProvider)
          .delete('$_uploads/$id')
          .then((_) {}, onError: (_) {}),
    );
  }

  void _cancel() {
    _cancelled = true;
    _cancelToken.cancel();
    _discard();
    Navigator.pop(context);
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final text = switch (_state) {
      'starting' => s.appUnzipStarting,
      'uploading' => s.appUnzipUploading(
        formatBytes(_sent),
        formatBytes(widget.file.size),
      ),
      'checking' => s.appUnzipChecking,
      'unpacking' => s.appUnzipUnpacking('${(_percent * 100).floor()}'),
      _ => _error ?? '',
    };
    final failed = _state == 'error';
    return AlertDialog(
      title: Text(widget.file.name),
      content: SizedBox(
        width: 440,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              s.appUnzipBody(widget.path),
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
            const SizedBox(height: 16),
            if (!failed)
              LinearProgressIndicator(
                value: _state == 'uploading' || _state == 'unpacking'
                    ? _percent.clamp(0, 1)
                    : null,
              ),
            const SizedBox(height: 10),
            Text(text, style: TextStyle(color: failed ? c.failed : null)),
          ],
        ),
      ),
      actions: [
        if (failed)
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: Text(s.actionClose),
          )
        else
          TextButton(onPressed: _cancel, child: Text(s.actionCancel)),
      ],
    );
  }
}
