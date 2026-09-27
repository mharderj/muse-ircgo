// Package emoji translates :shortcode: sequences — the form Discord
// bridges use for emotes — into Unicode emoji for display, and maps emoji
// back to their shortcode for the hover preview.
package emoji

import (
	"regexp"
	"sort"
	"strings"

	"ircgo/internal/link"
)

// shortcodes maps a shortcode name (without colons, lowercase) to its emoji.
// It covers the shortcodes Discord and GitHub accept; custom server emote
// names that collide with these render as the standard emoji.
var shortcodes = map[string]string{
	// Smileys
	"grinning": "😀", "smiley": "😃", "smile": "😄", "grin": "😁",
	"joy": "😂", "rofl": "🤣", "sweat_smile": "😅", "laughing": "😆",
	"satisfied": "😆", "innocent": "😇", "smiling_imp": "😈", "imp": "👿",
	"wink": "😉", "blush": "😊", "slightly_smiling_face": "🙂",
	"upside_down_face": "🙃", "relaxed": "☺️", "yum": "😋",
	"relieved": "😌", "heart_eyes": "😍", "sunglasses": "😎",
	"smirk": "😏", "neutral_face": "😐", "expressionless": "😑",
	"no_mouth": "😶", "unamused": "😒", "roll_eyes": "🙄",
	"thinking": "🤔", "lying_face": "🤥", "hand_over_mouth": "🤭",
	"shushing_face": "🤫", "symbols_over_mouth": "🤬",
	"exploding_head": "🤯", "flushed": "😳", "flushing": "😳",
	"scream": "😱", "fearful": "😨", "cold_sweat": "😰",
	"hot_face": "🥵", "cold_face": "🥶", "pleading_face": "🥺",
	"cry": "😢", "sob": "😭", "disappointed_relieved": "😥",
	"disappointed": "😞", "persevere": "😣", "confounded": "😖", "confused": "😕",
	"tired_face": "😫", "weary": "😩", "triumph": "😤",
	"rage": "😡", "pout": "😡", "angry": "😠",
	"kissing": "😗", "kissing_smiling_eyes": "😙",
	"kissing_closed_eyes": "😚", "kissing_heart": "😘",
	"stuck_out_tongue": "😛", "stuck_out_tongue_winking_eye": "😜",
	"stuck_out_tongue_closed_eyes": "😝", "zany_face": "🤪",
	"woozy_face": "🥴", "dizzy_face": "😵",
	"face_exhaling": "😮\u200d💨", "face_with_spiral_eyes": "😵\u200d💫",
	"saluting_face": "🫡", "melting_face": "🫠",
	"sleeping": "😴", "sleepy": "😪", "drooling_face": "🤤",
	"mask": "😷", "thermometer_face": "🤒", "face_with_thermometer": "🤒",
	"head_bandage": "🤕", "face_with_head_bandage": "🤕",
	"cowboy_hat_face": "🤠", "clown_face": "🤡",
	"disguised_face": "🥸", "monocle_face": "🧐", "nerd_face": "🤓",
	"smiling_face_with_tear": "🥲",
	// Cat smileys
	"smiley_cat": "😺", "smile_cat": "😸", "joy_cat": "😹",
	"heart_eyes_cat": "😻", "smirk_cat": "😼", "kissing_cat": "😽",
	"scream_cat": "🙀", "crying_cat_face": "😿", "pouting_cat": "😾",
	// Faces / people
	"skull": "💀", "skull_and_crossbones": "☠️",
	"ghost": "👻", "alien": "👾", "robot_face": "🤖", "robot": "🤖",
	"see_no_evil": "🙈", "hear_no_evil": "🙉", "speak_no_evil": "🙊",
	"ogre": "👹", "goblin": "👺", "poop": "💩", "hankey": "💩",
	"shit": "💩", "jack_o_lantern": "🎃", "santa": "🎅",
	"mrs_claus": "🤶", "angel": "👼", "princess": "👸",
	"superhero": "🦸", "supervillain": "🦹", "mage": "🧙",
	"fairy": "🧚", "vampire": "🧛", "mermaid": "🧜",
	"elf": "🧝", "genie": "🧞", "zombie": "🧟", "ninja": "🥷",
	"detective": "🕵️", "spy": "🕵️",
	"dancer": "💃", "woman_dancing": "💃",
	"levitate": "🕴️", "man_in_business_suit_levitating": "🕴️",
	"running": "🏃", "running_man": "🏃", "dancers": "👯",
	"no_good": "🙅", "ok_woman": "🙆", "bow": "🙇",
	"facepalm": "🤦",
	// Hands
	"wave": "👋", "raised_hand": "✋", "raised_back_of_hand": "🤚",
	"splayed_hand": "🖐️", "hand_splayed": "🖐️",
	"vulcan_salute": "🖖", "ok_hand": "👌",
	"pinched_fingers": "🤌", "pinching_hand": "🤏",
	"v": "✌️", "victory": "✌️",
	"crossed_fingers": "🤞", "love_you_gesture": "🤟",
	"metal": "🤘", "call_me_hand": "🤙",
	"point_left": "👈", "point_right": "👉",
	"point_up_2": "👆", "point_down": "👇", "point_up": "☝️",
	"fu": "🖕", "middle_finger": "🖕",
	"raised_hands": "🙌", "clap": "👏", "pray": "🙏",
	"handshake": "🤝", "muscle": "💪",
	"mechanical_arm": "🦾", "writing_hand": "✍️",
	"nail_care": "💅", "selfie": "🤳",
	"+1": "👍", "thumbsup": "👍", "-1": "👎", "thumbsdown": "👎",
	"fist": "✊", "fist_raised": "✊",
	"fist_oncoming": "👊", "facepunch": "👊", "punch": "👊",
	"fist_left": "🤛", "fist_right": "🤜",
	// Body
	"ear": "👂", "nose": "👃", "eyes": "👀", "eye": "👁️",
	"tongue": "👅", "lips": "👄", "brain": "🧠",
	"bust_in_silhouette": "👤", "busts_in_silhouette": "👥",
	"speaking_head": "🗣️", "speaking_head_in_silhouette": "🗣️",
	"footprints": "👣", "leg": "🦵", "foot": "🦶",
	// Hearts
	"heart": "❤️", "orange_heart": "🧡", "yellow_heart": "💛",
	"green_heart": "💚", "blue_heart": "💙", "purple_heart": "💜",
	"black_heart": "🖤", "white_heart": "🤍", "brown_heart": "🤎",
	"broken_heart": "💔", "heavy_heart_exclamation": "❣️",
	"heart_exclamation": "❣️", "two_hearts": "💕",
	"revolving_hearts": "💞", "heartbeat": "💓", "heartpulse": "💗",
	"sparkling_heart": "💖", "cupid": "💘", "gift_heart": "💝",
	"heart_decoration": "💟", "mending_heart": "❤️\u200d🩹",
	"heart_on_fire": "❤️\u200d🔥",
	// Symbols
	"fire": "🔥", "star": "⭐", "star2": "🌟", "sparkles": "✨",
	"sparkle": "❇️", "zap": "⚡", "boom": "💥", "collision": "💥",
	"dizzy": "💫", "sweat_drops": "💦", "droplet": "💧",
	"tada": "🎉", "confetti_ball": "🎊", "balloon": "🎈",
	"gift": "🎁", "ribbon": "🎀", "trophy": "🏆",
	"sports_medal": "🏅", "medal": "🏅",
	"1st_place_medal": "🥇", "2nd_place_medal": "🥈",
	"3rd_place_medal": "🥉", "military_medal": "🎖️",
	"100": "💯", "white_check_mark": "✅", "ballot_box_with_check": "☑️",
	"heavy_check_mark": "✔️", "x": "❌",
	"question": "❓", "grey_question": "❔",
	"exclamation": "❗", "heavy_exclamation_mark": "❗",
	"grey_exclamation": "❕", "interrobang": "⁉️",
	"warning": "⚠️", "no_entry_sign": "🚫", "prohibited": "🚫",
	"bulb": "💡", "idea": "💡", "speech_balloon": "💬",
	"thought_balloon": "💭", "anger": "💢", "zzz": "💤",
	"recycle": "♻️", "fleur_de_lis": "⚜️",
	"warning_sign": "⚠️",
	// Objects
	"bell": "🔔", "no_bell": "🔕", "mute": "🔕",
	"loud_sound": "🔊", "speaker": "🔈", "sound": "🔉",
	"microphone": "🎤", "mic": "🎤", "headphones": "🎧",
	"radio": "📻", "tv": "📺", "camera": "📷",
	"camera_flash": "📸", "video_camera": "📹",
	"film_projector": "📽️", "projector": "📽️",
	"game_die": "🎲", "dart": "🎯", "8ball": "🎱",
	"joystick": "🕹️", "video_game": "🎮",
	"performing_arts": "🎭", "art": "🎨",
	"musical_note": "🎵", "notes": "🎶",
	"guitar": "🎸", "drum": "🥁", "trumpet": "🎺",
	"saxophone": "🎷", "violin": "🎻", "banjo": "🪕",
	"moneybag": "💰", "dollar": "💵", "money_with_wings": "💸",
	"credit_card": "💳", "gem": "💎", "ring": "💍",
	"crown": "👑", "tophat": "🎩", "graduation_cap": "🎓",
	"briefcase": "💼", "package": "📦",
	"e-mail": "📧", "email": "📧", "envelope": "✉️",
	"incoming_envelope": "📨", "mailbox": "📫",
	"calendar": "📅", "alarm_clock": "⏰", "hourglass": "⌛",
	"hourglass_flowing_sand": "⏳", "watch": "⌚",
	"stopwatch": "⏱️", "timer_clock": "⏲️",
	"lock": "🔒", "locked": "🔒", "unlock": "🔓", "unlocked": "🔓",
	"key": "🔑", "hammer": "🔨", "wrench": "🔧",
	"gear": "⚙️", "scissors": "✂️", "link": "🔗",
	"paperclip": "📎", "pushpin": "📌",
	"round_pushpin": "📍", "mag": "🔍", "mag_right": "🔎",
	"book": "📖", "open_book": "📖", "books": "📚",
	"notebook": "📓", "pencil": "✏️", "memo": "📝",
	"computer": "💻", "keyboard": "⌨️",
	"telephone_receiver": "📞", "phone": "☎️",
	"battery": "🔋", "electric_plug": "🔌",
	"flashlight": "🔦", "candle": "🕯️",
	"trash": "🗑️", "wastebasket": "🗑️",
	"hammer_and_wrench": "🛠️",
	// Animals
	"dog": "🐶", "cat": "🐱", "mouse": "🐭", "hamster": "🐹",
	"rabbit": "🐰", "frog": "🐸", "monkey": "🐵",
	"pig": "🐷", "cow": "🐮", "panda_face": "🐼",
	"koala": "🐨", "tiger": "🐯", "lion_face": "🦁",
	"fox_face": "🦊", "wolf": "🐺", "bear": "🐻",
	"octopus": "🐙", "squid": "🦑", "butterfly": "🦋",
	"bee": "🐝", "honeybee": "🐝", "bug": "🐛",
	"snail": "🐌", "turtle": "🐢", "snake": "🐍",
	"dragon": "🐉", "whale": "🐳", "dolphin": "🐬",
	"fish": "🐟", "shark": "🦈", "crab": "🦀",
	"penguin": "🐧", "bird": "🐦", "chicken": "🐔",
	"owl": "🦉", "bat": "🦇",
	// Food & drink
	"pizza": "🍕", "hamburger": "🍔", "fries": "🍟",
	"hotdog": "🌭", "taco": "🌮", "burrito": "🌯",
	"popcorn": "🍿", "coffee": "☕", "tea": "🍵",
	"beer": "🍺", "beers": "🍻", "wine_glass": "🍷",
	"cocktail": "🍸", "champagne": "🍾", "tropical_drink": "🍹",
	"cake": "🍰", "birthday": "🎂", "cookie": "🍪",
	"doughnut": "🍩", "ice_cream": "🍨", "lollipop": "🍭",
	"candy": "🍬", "apple": "🍎", "banana": "🍌",
	"strawberry": "🍓", "watermelon": "🍉", "lemon": "🍋",
	"grapes": "🍇", "cherries": "🍒", "peach": "🍑",
	"corn": "🌽", "carrot": "🥕", "avocado": "🥑",
	"broccoli": "🥦", "mushroom": "🍄", "sandwich": "🥪",
	// Nature & weather
	"sunny": "☀️", "cloud": "☁️", "umbrella": "☂️",
	"snowflake": "❄️", "snowman": "⛄",
	"rainbow": "🌈", "ocean": "🌊", "water_wave": "🌊",
	"earth_americas": "🌎", "globe_with_meridians": "🌐",
	"moon": "🌙", "crescent_moon": "🌙", "full_moon": "🌕",
	"comet": "☄️", "cactus": "🌵", "sunflower": "🌻",
	"rose": "🌹", "four_leaf_clover": "🍀", "shamrock": "☘️",
	// Transport
	"car": "🚗", "red_car": "🚗", "taxi": "🚕", "bus": "🚌",
	"train": "🚂", "airplane": "✈️", "rocket": "🚀",
	"ship": "🚢", "bike": "🚲", "motorcycle": "🏍️",
	// Sport
	"soccer": "⚽", "basketball": "🏀", "football": "🏈",
	"baseball": "⚾", "tennis": "🎾", "volleyball": "🏐",
	"golf": "⛳", "fishing_pole_and_fish": "🎣",
	"ticket": "🎫",
	// Flags (common)
	"us": "🇺🇸", "uk": "🇬🇧", "gb": "🇬🇧", "ca": "🇨🇦",
	"de": "🇩🇪", "fr": "🇫🇷", "es": "🇪🇸", "it": "🇮🇹",
	"jp": "🇯🇵", "cn": "🇨🇳", "kr": "🇰🇷", "au": "🇦🇺",
	"pirate_flag": "🏴‍☠️",
}

// byEmoji maps an emoji back to its primary shortcode, for the hover
// preview. When several names map to one emoji, the first one wins.
var byEmoji = map[string]string{}

func init() {
	for name, e := range shortcodes {
		if _, ok := byEmoji[e]; !ok {
			byEmoji[e] = name
		}
	}
}

// Suggestion is one autocomplete match: the shortcode name and its emoji.
type Suggestion struct {
	Name  string
	Emoji string
}

// Suggest returns the shortcodes starting with prefix (case-insensitive),
// sorted by name. An empty prefix returns everything, so typing just ":"
// opens the full list.
func Suggest(prefix string) []Suggestion {
	prefix = strings.ToLower(prefix)
	var out []Suggestion
	for name, e := range shortcodes {
		if strings.HasPrefix(name, prefix) {
			out = append(out, Suggestion{Name: name, Emoji: e})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var shortcodeRe = regexp.MustCompile(`:([A-Za-z0-9_+\-]{2,}):`)

// Replace converts :shortcode: sequences in s to their emoji. URLs are
// left untouched so links never break, and unknown shortcodes are left
// as-is. Matching is case-insensitive.
func Replace(s string) string {
	idxs := shortcodeRe.FindAllStringSubmatchIndex(s, -1)
	if len(idxs) == 0 {
		return s
	}
	spans := link.FindSpans(s)
	var b strings.Builder
	pos := 0
	for _, idx := range idxs {
		start, end := idx[0], idx[1]
		name := strings.ToLower(s[idx[2]:idx[3]])
		e, ok := shortcodes[name]
		if !ok || insideSpans(spans, start, end) {
			continue
		}
		b.WriteString(s[pos:start])
		b.WriteString(e)
		pos = end
	}
	b.WriteString(s[pos:])
	return b.String()
}

func insideSpans(spans []link.Span, start, end int) bool {
	for _, sp := range spans {
		if start >= sp.Start && end <= sp.End {
			return true
		}
	}
	return false
}

// Lookup returns the shortcode for an emoji, if it has one.
func Lookup(e string) (string, bool) {
	name, ok := byEmoji[e]
	return name, ok
}

// maxEmojiRunes bounds how many runes one mapped emoji can hold
// (ZWJ sequences); At never looks further ahead than this.
const maxEmojiRunes = 8

// At reports the emoji starting at runes[i] and its shortcode, trying the
// longest match first. It reports false when no known emoji starts there.
func At(runes []rune, i int) (emoji, shortcode string, ok bool) {
	if i < 0 || i >= len(runes) {
		return "", "", false
	}
	end := i + maxEmojiRunes
	if end > len(runes) {
		end = len(runes)
	}
	for j := end; j > i; j-- {
		cand := string(runes[i:j])
		if name, found := byEmoji[cand]; found {
			return cand, name, true
		}
	}
	return "", "", false
}
