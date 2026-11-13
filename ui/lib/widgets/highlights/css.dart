import 'package:material_ui/material_ui.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final cssSyntaxRules = <SyntaxRule>[
  // 1. Comments
  SyntaxRule(
    RegExp(r'/\*[\s\S]*?\*/'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // 2. Strings
  SyntaxRule(
    RegExp(r'''"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*' '''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // 3. At-rules
  SyntaxRule(
    RegExp(r'@[a-zA-Z-]+'),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // 4. CSS variables
  SyntaxRule(
    RegExp(r'--[a-zA-Z_][\w-]*'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // 5. Selectors: IDs
  SyntaxRule(
    RegExp(r'#[a-zA-Z_][\w-]*'),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // 6. Selectors: classes
  SyntaxRule(
    RegExp(r'\.[a-zA-Z_][\w-]*'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // 7. Pseudo-elements
  SyntaxRule(
    RegExp(r'::[a-zA-Z-]+'),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // 8. Pseudo-classes
  SyntaxRule(
    RegExp(r':[a-zA-Z-]+(?:\([^)]*\))?'),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // 9. CSS functions
  SyntaxRule(
    RegExp(
      r'\b(?:rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch|'
      r'var|calc|min|max|clamp|url|attr|counter|counters|'
      r'linear-gradient|radial-gradient|conic-gradient|'
      r'repeating-linear-gradient|repeating-radial-gradient|'
      r'transform|translate|rotate|scale|matrix|'
      r'blur|brightness|contrast|drop-shadow|'
      r'cubic-bezier|steps|repeat|minmax|fit-content|'
      r'light-dark|color-mix|color|env|'
      r'format|local|element|paint|image-set)\b(?=\s*\()',
    ),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // 10. Important declaration
  SyntaxRule(
    RegExp(r'!\s*important\b'),
    const TextStyle(color: Color(0xFFE06C75), fontWeight: FontWeight.bold),
  ),

  // 11. CSS colors: hex
  SyntaxRule(
    RegExp(r'#[0-9a-fA-F]{3,8}\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // 12. CSS color keywords
  SyntaxRule(
    RegExp(
      r'\b(?:transparent|currentColor|inherit|initial|unset|'
      r'revert|revert-layer|red|blue|green|white|black|'
      r'yellow|orange|purple|pink|gray|grey|silver|navy|'
      r'teal|aqua|lime|fuchsia|maroon|olive|coral|tomato|'
      r'indigo|violet|gold|crimson|salmon|beige|brown)\b',
      caseSensitive: false,
    ),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // 13. Numbers and CSS units
  SyntaxRule(
    RegExp(
      r'(?<![\w-])(?:\d*\.)?\d+(?:%|px|em|rem|vh|vw|'
      r'vmin|vmax|svh|svw|lvh|lvw|dvh|dvw|ch|ex|'
      r'cm|mm|in|pt|pc|deg|rad|turn|s|ms|fr)?\b',
    ),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // 14. Property names
  SyntaxRule(
    RegExp(r'(?<=\s)[a-zA-Z-]+(?=\s*:)'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // 15. Common CSS keywords
  SyntaxRule(
    RegExp(
      r'\b(?:none|auto|block|inline|inline-block|flex|grid|'
      r'inline-flex|inline-grid|absolute|relative|fixed|sticky|'
      r'static|solid|dashed|dotted|double|hidden|visible|'
      r'collapse|bold|normal|italic|lighter|bolder|'
      r'center|left|right|justify|top|bottom|middle|'
      r'cover|contain|repeat|no-repeat|space-between|'
      r'space-around|space-evenly|pointer|default|'
      r'nowrap|wrap|wrap-reverse|row|column|row-reverse|'
      r'column-reverse|uppercase|lowercase|capitalize|'
      r'break-word|border-box|content-box|ease|linear|'
      r'ease-in|ease-out|ease-in-out|forwards|backwards|'
      r'infinite|alternate|running|paused|screen|print|'
      r'and|or|not|only)\b',
    ),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // 16. HTML element selectors
  SyntaxRule(
    RegExp(r'(?<![\w-])[a-zA-Z][a-zA-Z0-9-]*(?=\s*(?:[.#:\[,{>+~]|$))'),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // 17. Operators and punctuation
  SyntaxRule(
    RegExp(r'[{}()[\];:,>+~*=|^$]'),
    const TextStyle(color: Color(0xFFABB2BF)),
  ),
];
