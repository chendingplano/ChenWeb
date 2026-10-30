## MODIFIED Requirements

### Requirement: Interpreter text is translated
The interpreter SHALL take its own UI text from Paraglide messages in English and Chinese,
including prompts, errors, loading/empty states, text help controls and text Properties.
Supported localized descriptor fields SHALL select current locale then English. This
includes text-module static content, caption and tooltip as specified by `wpi-text-element`.
Plain strings SHALL be shown as written; bound objects SHALL retain String conversion
rather than being interpreted as language maps. Other module-specific text retains its
existing contract.

#### Scenario: Chinese locale
- **WHEN** the locale is `zh-cn` and a text element has English/Chinese static content, caption and tooltip
- **THEN** its content and supporting text use Chinese and its help control has a Chinese accessible name

#### Scenario: Plain strings remain literal
- **WHEN** a text element's source is a plain string or a bound object
- **THEN** the string is not translated and the bound object is not interpreted as a language map
