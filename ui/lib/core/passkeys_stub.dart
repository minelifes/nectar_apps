bool supported() => false;

Future<Map<String, dynamic>> create(Map<String, dynamic> options) =>
    throw UnsupportedError('passkeys need the web panel');

Future<Map<String, dynamic>> get(Map<String, dynamic> options) =>
    throw UnsupportedError('passkeys need the web panel');
