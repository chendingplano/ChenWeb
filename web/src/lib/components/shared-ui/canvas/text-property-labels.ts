import { m } from '$lib/paraglide/messages.js';

export function textAttributeLabel(name: string): string {
	switch (name) {
		case 'text':
			return m.wpi_text_attr_text();
		case 'data_binding':
			return m.wpi_text_attr_data_binding();
		case 'label':
			return m.wpi_text_attr_label();
		case 'help_tip':
			return m.wpi_text_attr_help_tip();
		case 'icon':
			return m.wpi_text_attr_icon();
		case 'label_placement':
			return m.wpi_text_attr_label_placement();
		case 'label_horizontal_gap':
			return m.wpi_text_attr_label_horizontal_gap();
		case 'label_vertical_gap':
			return m.wpi_text_attr_label_vertical_gap();
		case 'label_horizontal_align':
			return m.wpi_text_attr_label_horizontal_align();
		case 'label_vertical_align':
			return m.wpi_text_attr_label_vertical_align();
		case 'label_max_width':
			return m.wpi_text_attr_label_max_width();
		case 'label_font_weight':
			return m.wpi_text_attr_label_font_weight();
		case 'icon_placement':
			return m.wpi_text_attr_icon_placement();
		case 'icon_size':
			return m.wpi_text_attr_icon_size();
		case 'icon_gap':
			return m.wpi_text_attr_icon_gap();
		case 'icon_color':
			return m.wpi_text_attr_icon_color();
		case 'font_family':
			return m.wpi_text_attr_font_family();
		case 'font_size':
			return m.wpi_text_attr_font_size();
		case 'font_weight':
			return m.wpi_text_attr_font_weight();
		case 'font_style':
			return m.wpi_text_attr_font_style();
		case 'line_height':
			return m.wpi_text_attr_line_height();
		case 'color':
			return m.wpi_text_attr_color();
		case 'background_color':
			return m.wpi_text_attr_background_color();
		case 'text_align':
			return m.wpi_text_attr_text_align();
		case 'white_space':
			return m.wpi_text_attr_white_space();
		case 'padding':
			return m.wpi_text_attr_padding();
		case 'width':
			return m.wpi_text_attr_width();
		case 'height':
			return m.wpi_text_attr_height();
		case 'min_width':
			return m.wpi_text_attr_min_width();
		case 'max_width':
			return m.wpi_text_attr_max_width();
		default:
			return name;
	}
}

export function textChoiceLabel(name: string): string {
	switch (name) {
		case '':
			return m.wpi_text_choice_none();
		case 'info':
			return m.wpi_text_choice_info();
		case 'circle-help':
			return m.wpi_text_choice_circle_help();
		case 'circle-check':
			return m.wpi_text_choice_circle_check();
		case 'triangle-alert':
			return m.wpi_text_choice_triangle_alert();
		case 'file-text':
			return m.wpi_text_choice_file_text();
		case 'user':
			return m.wpi_text_choice_user();
		case 'top':
			return m.wpi_text_choice_top();
		case 'left':
			return m.wpi_text_choice_left();
		case 'right':
			return m.wpi_text_choice_right();
		case 'bottom':
			return m.wpi_text_choice_bottom();
		case 'start':
			return m.wpi_text_choice_start();
		case 'center':
			return m.wpi_text_choice_center();
		case 'end':
			return m.wpi_text_choice_end();
		case 'normal':
			return m.wpi_text_choice_normal();
		case 'bold':
			return m.wpi_text_choice_bold();
		case 'inherit':
			return m.wpi_text_choice_inherit();
		case 'italic':
			return m.wpi_text_choice_italic();
		case 'oblique':
			return m.wpi_text_choice_oblique();
		case 'justify':
			return m.wpi_text_choice_justify();
		case 'pre-wrap':
			return m.wpi_text_choice_pre_wrap();
		default:
			return name;
	}
}

export function textGroupLabel(name: string): string {
	switch (name) {
		case 'content':
			return m.wpi_text_group_content();
		case 'caption_layout':
			return m.wpi_text_group_caption_layout();
		case 'icon':
			return m.wpi_text_group_icon();
		case 'typography':
			return m.wpi_text_group_typography();
		case 'appearance':
			return m.wpi_text_group_appearance();
		case 'size':
			return m.wpi_text_group_size();
		default:
			return m.wpi_text_group_other();
	}
}

export function textGroupHelp(name: string): string {
	switch (name) {
		case 'content':
			return m.wpi_text_hint_content();
		case 'caption_layout':
			return m.wpi_text_hint_caption();
		case 'icon':
			return m.wpi_text_hint_icon();
		case 'typography':
			return m.wpi_text_hint_typography();
		case 'appearance':
			return m.wpi_text_hint_appearance();
		case 'size':
			return m.wpi_text_hint_size();
		default:
			return '';
	}
}
