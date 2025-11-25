// types/menu.ts
import type { Icon } from "@tabler/icons-svelte";

export type BasicMenuItem = {
    id:         string;
    title:      string;
    url?:       string;
    icon?:      Icon;
};

export type NullMenuItem = BasicMenuItem & {
    type: "null"
}

export type TeamMenuItem = BasicMenuItem & {
    type: "team";
    plan: string
}

export type NavigationMenuItem = BasicMenuItem & {
    type: "navigation"
}

export type UserMenuItem = BasicMenuItem & {
    type:   "user",
    name:   string; 
    email:  string; 
    avatar: string
}
export type ModalItem = BasicMenuItem & {
    type:       'modal';
    modalId:    string;
    modalData?: any;
};

export type ApiItem = BasicMenuItem & {
    type:       'api';
    endpoint:   string;
    method:     'GET' | 'POST' | 'DELETE';
};

export type Sidebar01MenuItem = NavigationMenuItem | NullMenuItem | UserMenuItem;
export type Sidebar07MenuItem = NavigationMenuItem | NullMenuItem | UserMenuItem | TeamMenuItem;