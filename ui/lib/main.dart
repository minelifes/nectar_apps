import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  _registerFontLicenses();
  // Errors are shown where they happen (pages show a retry); do not retry failed requests automatically.
  runApp(ProviderScope(retry: (retryCount, error) => null, child: const NectarApp()));
}

void _registerFontLicenses() {
  LicenseRegistry.addLicense(() async* {
    for (final (family, file) in const [
      ('Inter', 'LICENSE-Inter.txt'),
      ('Space Grotesk', 'LICENSE-SpaceGrotesk.txt'),
      ('JetBrains Mono', 'LICENSE-JetBrainsMono.txt'),
    ]) {
      yield LicenseEntryWithLineBreaks([family], await rootBundle.loadString('assets/fonts/$file'));
    }
  });
}
