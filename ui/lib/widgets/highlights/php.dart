import 'package:material_ui/material_ui.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final phpSyntaxRules = <SyntaxRule>[
  // PHP opening and closing tags
  SyntaxRule(
    RegExp(r'<\?(?:php|=)?|\?>'),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Comments
  SyntaxRule(
    RegExp(r'//[^\n]*|#[^\n]*|/\*[\s\S]*?\*/'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // Strings
  SyntaxRule(
    RegExp(r"""(?:"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')"""),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Variables
  SyntaxRule(
    RegExp(r'\$[a-zA-Z_][a-zA-Z0-9_]*'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // PHP keywords
  SyntaxRule(
    RegExp(
      r'\b(abstract|and|array|as|break|callable|case|catch|'
      r'class|clone|const|continue|declare|default|die|do|'
      r'echo|else|elseif|empty|endfor|endforeach|endif|'
      r'endswitch|endwhile|enum|eval|exit|extends|final|'
      r'finally|for|foreach|function|global|goto|if|'
      r'implements|include|include_once|instanceof|insteadof|'
      r'interface|isset|list|match|namespace|new|or|print|'
      r'private|protected|public|readonly|require|require_once|'
      r'return|static|switch|throw|trait|try|unset|use|'
      r'var|while|xor|yield|yield from)\b',
      caseSensitive: false,
    ),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Types
  SyntaxRule(
    RegExp(
      r'\b(int|float|string|bool|boolean|void|mixed|never|'
      r'object|iterable|self|parent|null|false|true)\b',
    ),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Special constants
  SyntaxRule(
    RegExp(r'\b[A-Z][A-Z0-9_]+\b'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Class names after class/interface/trait/enum/new
  SyntaxRule(
    RegExp(r'\b[A-Z][a-zA-Z0-9_]*\b'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Function calls
  SyntaxRule(
    RegExp(r'\b[a-zA-Z_][a-zA-Z0-9_]*(?=\s*\()'),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // Numbers
  SyntaxRule(
    RegExp(r'\b(?:0x[0-9a-fA-F]+|\d+(?:\.\d+)?)\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Operators
  SyntaxRule(
    RegExp(
      r'===|!==|=>|->|\?\?|\?->|==|!=|<=|>=|&&|\|\||\+\+|--|'
      r'\+=|-=|\*=|/=|%=|[+\-*/%=<>!&|.^~?:]',
    ),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // Punctuation
  SyntaxRule(
    RegExp(r'[{}()\[\];,]'),
    const TextStyle(color: Color(0xFFABB2BF)),
  ),
];
