import sys
import copy
from PySide6.QtCore import Qt, QRectF
from PySide6.QtGui import QColor, QLinearGradient, QPainter, QBrush, QPen
from PySide6.QtWidgets import (
    QApplication, QCheckBox, QColorDialog, QComboBox, QDialog, QFileDialog,
    QFormLayout, QHBoxLayout, QHeaderView, QLabel, QLineEdit, QListWidget,
    QMainWindow, QMessageBox, QPushButton, QSlider, QDoubleSpinBox, QSplitter,
    QTableWidget, QTableWidgetItem, QVBoxLayout, QSizePolicy, QWidget,
)

try:
    import tomllib
except ImportError:
    try:
        import tomli as tomllib
    except ImportError:
        tomllib = None


def _format_inline_table(d):
    # helper to format dictionary into inline toml table with trailing commas
    parts = []
    for k, v in d.items():
        if isinstance(v, bool):
            val_str = "true" if v else "false"
        elif isinstance(v, str):
            val_str = f'"{v}"'
        elif isinstance(v, dict):
            val_str = _format_inline_table(v)
        elif isinstance(v, list):
            items = [_format_inline_table(item) if isinstance(item, dict) else str(item) for item in v]
            val_str = f"[{', '.join(items)}]"
        else:
            val_str = str(v)
        parts.append(f"{k} = {val_str},")
    return f"{{ {' '.join(parts)} }}"


def dump_scenes_toml(data, path):
    # write toml matching scenes.toml dsl formatting rules
    lines = [
        "# SceneScript is a declarative DSL to define scenes for WiZ-CLI Control",
        "# (only the [SCENE] section will get evaluated)",
        "",
        "[SCENES]",
        ""
    ]
    scenes = data.get("scenes", {})
    for key, scene in scenes.items():
        # wrap key in quotes to support names starting with numbers or symbols
        lines.append(f'[scenes."{key}"]')
        desc = scene.get("description", "")
        lines.append(f'description = "{desc}"')
        loop_val = "true" if scene.get("loop", False) else "false"
        lines.append(f"loop = {loop_val}")
        lines.append("actions = [")
        
        actions = scene.get("actions", [])
        action_strings = []
        for act in actions:
            act_str = _format_inline_table(act)
            action_strings.append(f"    {act_str}")
        
        if action_strings:
            lines.append(",\n".join(action_strings))
        lines.append("]")
        lines.append("")
    
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))


class VisualLightPreviewBar(QWidget):
    """Custom graphical widget that paints a continuous linear visual gradient 
    representing the light color sequence over time."""

    def __init__(self, parent=None):
        super().__init__(parent)
        self.setFixedHeight(30)
        self.colors = [QColor(255, 0, 0), QColor(0, 255, 200), QColor(164, 0, 255)]

    def set_colors(self, qcolors):
        if qcolors:
            self.colors = qcolors
        else:
            self.colors = [QColor(50, 50, 50)]
        self.update()

    def paintEvent(self, event):
        painter = QPainter(self)
        painter.setRenderHint(QPainter.RenderHint.Antialiasing)

        rect = QRectF(0, 0, self.width(), self.height())
        if not self.colors:
            painter.fillRect(rect, QColor(30, 30, 30))
            return

        gradient = QLinearGradient(0, 0, self.width(), 0)
        if len(self.colors) == 1:
            gradient.setColorAt(0.0, self.colors[0])
            gradient.setColorAt(1.0, self.colors[0])
        else:
            count = len(self.colors)
            for idx, col in enumerate(self.colors):
                pos = idx / (count - 1)
                gradient.setColorAt(pos, col)

        painter.setBrush(QBrush(gradient))
        painter.setPen(QPen(QColor(100, 100, 100), 1))
        painter.drawRoundedRect(rect, 5, 5)


class ColorSwatchButton(QPushButton):
    """Button showing a color preview; opens native GUI color picker when clicked."""

    def __init__(self, initial_color=QColor(255, 255, 255), parent=None):
        super().__init__(parent)
        self.color = initial_color
        self.setFixedHeight(30)
        self.clicked.connect(self._pick_color)
        self._update_style()

    def _update_style(self):
        r, g, b = self.color.red(), self.color.green(), self.color.blue()
        # Luminance calculation for readable text contrast
        luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255
        text_col = "#000000" if luminance > 0.5 else "#FFFFFF"
        self.setText(f"rgb = ({r}, {g}, {b})")
        self.setStyleSheet(
            f"background-color: rgb({r},{g},{b}); color: {text_col}; "
            f"border: 1px solid #777; border-radius: 4px; font-weight: bold;"
        )

    def _pick_color(self):
        new_col = QColorDialog.getColor(self.color, self, "Select Color")
        if new_col.isValid():
            self.color = new_col
            self._update_style()

    def get_rgb_dict(self):
        return {
            "r": self.color.red(),
            "g": self.color.green(),
            "b": self.color.blue(),
        }


class GraphicalActionDialog(QDialog):
    """Visual dialog color pickers, visual sliders, and time inputs."""

    def __init__(self, parent=None, action_data=None):
        super().__init__(parent)
        self.setWindowTitle("Graphical Action Builder")
        self.resize(450, 300)
        self.result_data = None

        layout = QVBoxLayout(self)

        # Method Selector
        layout.addWidget(QLabel("Action Method:"))
        self.method_cb = QComboBox()
        self.method_cb.addItems(
            [
                "setColor",
                "fadeTo",
                "delay",
                "setWhite",
                "fadeSequence",
                "setOff",
                "setOn",
            ]
        )
        layout.addWidget(self.method_cb)

        # Dynamic Visual Controls Container
        self.form_container = QWidget()
        self.form_layout = QFormLayout(self.form_container)
        layout.addWidget(self.form_container)

        self.method_cb.currentTextChanged.connect(self._rebuild_form)

        if action_data:
            self.method_cb.setCurrentText(action_data.get("method", "setColor"))
            self._rebuild_form(action_data.get("method", "setColor"), action_data)
        else:
            self._rebuild_form("setColor")

        # Dialog Buttons
        btn_layout = QHBoxLayout()
        btn_save = QPushButton("Save Action")
        btn_cancel = QPushButton("Cancel")
        btn_save.clicked.connect(self._on_save)
        btn_cancel.clicked.connect(self.reject)
        btn_layout.addStretch()
        btn_layout.addWidget(btn_cancel)
        btn_layout.addWidget(btn_save)
        layout.addLayout(btn_layout)

    def _rebuild_form(self, method, data=None):
        while self.form_layout.rowCount() > 0:
            self.form_layout.removeRow(0)

        data = data or {}

        if method in ("setColor", "fadeTo"):
            rgb = data.get("rgb", {}) if "rgb" in data else data
            init_col = QColor(
                rgb.get("r", 255), rgb.get("g", 255), rgb.get("b", 255)
            )
            self.swatch_btn = ColorSwatchButton(init_col)
            
            color_layout = QVBoxLayout()
            color_layout.addWidget(QLabel("Target Color:"))
            color_layout.addWidget(self.swatch_btn)
            self.form_layout.addRow(color_layout)

            self.bright_slider = QSlider(Qt.Orientation.Horizontal)
            self.bright_slider.setRange(0, 100)
            self.bright_slider.setValue(data.get("brightness", 100))
            self.bright_lbl = QLabel(f"{self.bright_slider.value()}%")
            self.bright_slider.valueChanged.connect(
                lambda v: self.bright_lbl.setText(f"{v}%")
            )

            bright_header = QHBoxLayout()
            bright_header.addWidget(QLabel("Brightness:"))
            bright_header.addStretch()
            bright_header.addWidget(self.bright_lbl)

            bright_layout = QVBoxLayout()
            bright_layout.addLayout(bright_header)
            bright_layout.addWidget(self.bright_slider)
            self.form_layout.addRow(bright_layout)

        if method in ("fadeTo", "delay"):
            max_ms = 60000 if method == "fadeTo" else 3600000
            default_ms = data.get("ms", 1500 if method == "fadeTo" else 1000)

            self.time_unit_cb = QComboBox()
            self.time_unit_cb.addItems(["ms", "s"])

            self.time_spin = QDoubleSpinBox()
            self.time_spin.setRange(0, max_ms)
            self.time_spin.setValue(default_ms)
            self.time_spin.setDecimals(0)
            self.time_spin.setSingleStep(100 if method == "fadeTo" else 250)

            def on_unit_changed(unit):
                curr_val = self.time_spin.value()
                if unit == "s":
                    self.time_spin.setDecimals(1)
                    self.time_spin.setSingleStep(0.5 if method == "fadeTo" else 1.0)
                    self.time_spin.setRange(0.0, max_ms / 1000.0)
                    self.time_spin.setValue(curr_val / 1000.0)
                else:
                    self.time_spin.setDecimals(0)
                    self.time_spin.setSingleStep(100 if method == "fadeTo" else 250)
                    self.time_spin.setRange(0, max_ms)
                    self.time_spin.setValue(curr_val * 1000.0)

            self.time_unit_cb.currentTextChanged.connect(on_unit_changed)

            header_layout = QHBoxLayout()
            label_text = "Transition Time:" if method == "fadeTo" else "Delay:"
            header_layout.addWidget(QLabel(label_text))
            header_layout.addStretch()
            header_layout.addWidget(QLabel("Unit:"))
            header_layout.addWidget(self.time_unit_cb)

            time_layout = QVBoxLayout()
            time_layout.addLayout(header_layout)
            time_layout.addWidget(self.time_spin)
            self.form_layout.addRow(time_layout)

        elif method == "setWhite":
            self.temp_slider = QSlider(Qt.Orientation.Horizontal)
            self.temp_slider.setRange(2200, 6500)
            self.temp_slider.setValue(data.get("temp", 2700))
            self.temp_lbl = QLabel(f"{self.temp_slider.value()} K")
            self.temp_slider.valueChanged.connect(
                lambda v: self.temp_lbl.setText(f"{v} K")
            )

            temp_header = QHBoxLayout()
            temp_header.addWidget(QLabel("Color Temp:"))
            temp_header.addStretch()
            temp_header.addWidget(self.temp_lbl)

            temp_layout = QVBoxLayout()
            temp_layout.addLayout(temp_header)
            temp_layout.addWidget(self.temp_slider)
            self.form_layout.addRow(temp_layout)

            self.bright_slider = QSlider(Qt.Orientation.Horizontal)
            self.bright_slider.setRange(0, 100)
            self.bright_slider.setValue(data.get("brightness", 100))
            self.bright_lbl = QLabel(f"{self.bright_slider.value()}%")
            self.bright_slider.valueChanged.connect(
                lambda v: self.bright_lbl.setText(f"{v}%")
            )

            bright_header = QHBoxLayout()
            bright_header.addWidget(QLabel("Brightness:"))
            bright_header.addStretch()
            bright_header.addWidget(self.bright_lbl)

            bright_layout = QVBoxLayout()
            bright_layout.addLayout(bright_header)
            bright_layout.addWidget(self.bright_slider)
            self.form_layout.addRow(bright_layout)

        elif method in ("setOff", "setOn"):
            self.form_layout.addRow(
                QLabel("<i>No additional parameters required.</i>")
            )

    def _on_save(self):
        method = self.method_cb.currentText()
        res = {"method": method}

        if method == "setColor":
            res["rgb"] = self.swatch_btn.get_rgb_dict()
            res["brightness"] = self.bright_slider.value()
            res["cw"] = {"c": 0, "w": 0}
        elif method in ("fadeTo", "delay"):
            val = self.time_spin.value()
            if self.time_unit_cb.currentText() == "s":
                res["ms"] = int(round(val * 1000))
            else:
                res["ms"] = int(round(val))
            if method == "fadeTo":
                rgb = self.swatch_btn.get_rgb_dict()
                res.update(rgb)
                res["brightness"] = self.bright_slider.value()
        elif method == "setWhite":
            res["temp"] = self.temp_slider.value()
            res["brightness"] = self.bright_slider.value()

        self.result_data = res
        self.accept()


class SceneScriptVisualApp(QMainWindow):
    def __init__(self):
        super().__init__()
        self.setWindowTitle("SceneScript Graphical Editor")
        self.resize(950, 650)

        self.data = {"scenes": {}}
        self.current_scene_key = None

        self._build_menu()
        self._build_ui()
        self._load_default_template()

    def _build_menu(self):
        menubar = self.menuBar()
        file_menu = menubar.addMenu("File")

        save_act = file_menu.addAction("Save SceneScript File...")
        save_act.triggered.connect(self.save_toml)

        file_menu.addSeparator()
        exit_act = file_menu.addAction("Exit")
        exit_act.triggered.connect(self.close)

    def _build_ui(self):
        splitter = QSplitter(Qt.Orientation.Horizontal)
        self.setCentralWidget(splitter)

        # Left Panel: Scene Selection
        left_widget = QWidget()
        left_layout = QVBoxLayout(left_widget)
        left_layout.addWidget(QLabel("<b>Scenes List</b>"))

        self.scene_list = QListWidget()
        self.scene_list.currentTextChanged.connect(self._on_scene_selected)
        left_layout.addWidget(self.scene_list)

        scene_btn_layout = QHBoxLayout()
        btn_add = QPushButton("+ Add Scene")
        btn_del = QPushButton("- Delete")
        btn_add.clicked.connect(self.add_scene)
        btn_del.clicked.connect(self.delete_scene)
        scene_btn_layout.addWidget(btn_add)
        scene_btn_layout.addWidget(btn_del)
        left_layout.addLayout(scene_btn_layout)

        splitter.addWidget(left_widget)

        # Right Panel: Graphical Canvas & Sequence Builder
        right_widget = QWidget()
        right_layout = QVBoxLayout(right_widget)

        # Top Light Visual Preview Canvas
        right_layout.addWidget(QLabel("<b>Visual Light Output Gradient</b>"))
        self.preview_bar = VisualLightPreviewBar()
        right_layout.addWidget(self.preview_bar)

        # Metadata Controls
        meta_form = QFormLayout()
        meta_form.setFieldGrowthPolicy(QFormLayout.FieldGrowthPolicy.AllNonFixedFieldsGrow)
        self.name_edit = QLineEdit()
        self.name_edit.setSizePolicy(QSizePolicy.Policy.Expanding, QSizePolicy.Policy.Fixed)
        self.name_edit.editingFinished.connect(self._on_name_changed)
        self.desc_edit = QLineEdit()
        self.desc_edit.setSizePolicy(QSizePolicy.Policy.Expanding, QSizePolicy.Policy.Fixed)
        self.desc_edit.textChanged.connect(self._on_desc_changed)
        self.loop_cb = QCheckBox()
        self.loop_cb.toggled.connect(self._on_loop_changed)
        meta_form.addRow("<b>Scene Name:<b>", self.name_edit)
        meta_form.addRow("Description:", self.desc_edit)
        meta_form.addRow("Loop Scene:", self.loop_cb)
        right_layout.addLayout(meta_form)

        # Visual Action Sequence Table
        right_layout.addWidget(QLabel("<b>Action Sequence Timeline</b>"))
        self.actions_table = QTableWidget(0, 3)
        self.actions_table.setEditTriggers(QTableWidget.EditTrigger.NoEditTriggers)
        self.actions_table.setHorizontalHeaderLabels(
            ["Method", "Visual", "Parameters"]
        )
        self.actions_table.horizontalHeader().setSectionResizeMode(
            2, QHeaderView.ResizeMode.Stretch
        )
        self.actions_table.setSelectionBehavior(
            QTableWidget.SelectionBehavior.SelectRows
        )
        right_layout.addWidget(self.actions_table)

        act_btn_layout = QHBoxLayout()
        btn_add_act = QPushButton("+ Add Action")
        btn_dup_act = QPushButton("Duplicate Action")
        btn_edit_act = QPushButton("Edit Action")
        btn_del_act = QPushButton("- Remove Action")
        btn_up_act = QPushButton("▲")
        btn_down_act = QPushButton("▼")

        btn_add_act.clicked.connect(self.add_action)
        btn_dup_act.clicked.connect(self.duplicate_action)
        btn_edit_act.clicked.connect(self.edit_action)
        btn_del_act.clicked.connect(self.remove_action)
        btn_up_act.clicked.connect(self.move_action_up)
        btn_down_act.clicked.connect(self.move_action_down)

        act_btn_layout.addWidget(btn_add_act)
        act_btn_layout.addWidget(btn_dup_act)
        act_btn_layout.addWidget(btn_edit_act)
        act_btn_layout.addWidget(btn_del_act)
        act_btn_layout.addWidget(btn_up_act)
        act_btn_layout.addWidget(btn_down_act)
        act_btn_layout.addStretch()
        right_layout.addLayout(act_btn_layout)

        splitter.addWidget(right_widget)
        splitter.setSizes([250, 700])

    def _load_default_template(self):
        self.data = {
            "scenes": {
                "aurora_pulse": {
                    "description": "Smooth gradient pulse through teal & purple",
                    "loop": True,
                    "actions": [
                        {"method": "setOff"},
                        {"method": "delay", "ms": 1000},
                        {
                            "method": "fadeTo",
                            "r": 0,
                            "g": 224,
                            "b": 208,
                            "brightness": 100,
                            "ms": 2500,
                        },
                        {
                            "method": "fadeTo",
                            "r": 164,
                            "g": 0,
                            "b": 255,
                            "brightness": 100,
                            "ms": 2500,
                        },
                    ],
                }
            }
        }
        self.refresh_scene_list()

    def refresh_scene_list(self):
        self.scene_list.blockSignals(True)
        self.scene_list.clear()
        for key in self.data.get("scenes", {}):
            self.scene_list.addItem(key)
        self.scene_list.blockSignals(False)

        if self.data.get("scenes"):
            self.scene_list.setCurrentRow(0)

    def _on_scene_selected(self, key):
        if not key or key not in self.data.get("scenes", {}):
            return
        self.current_scene_key = key
        scene = self.data["scenes"][key]

        self.name_edit.blockSignals(True)
        self.desc_edit.blockSignals(True)
        self.loop_cb.blockSignals(True)

        self.name_edit.setText(key)
        self.desc_edit.setText(scene.get("description", ""))
        self.loop_cb.setChecked(scene.get("loop", False))

        self.name_edit.blockSignals(False)
        self.desc_edit.blockSignals(False)
        self.loop_cb.blockSignals(False)

        self.refresh_actions_table()

    def refresh_actions_table(self):
        self.actions_table.setRowCount(0)
        if not self.current_scene_key:
            return

        actions = self.data["scenes"][self.current_scene_key].get("actions", [])
        extracted_qcolors = []

        for row, act in enumerate(actions):
            self.actions_table.insertRow(row)
            method = act.get("method", "")
            summary = ", ".join(
                f"{k}={v}" for k, v in act.items() if k != "method"
            )

            # Extract color for visual preview bar and visual table swatch
            col_chip = QLabel()
            #col_chip.setFixedSize(50, 50)

            extracted_color = None
            if "rgb" in act:
                r, g, b = act["rgb"]["r"], act["rgb"]["g"], act["rgb"]["b"]
                extracted_color = QColor(r, g, b)
            elif "r" in act and "g" in act and "b" in act:
                extracted_color = QColor(act["r"], act["g"], act["b"])
            elif method == "setWhite":
                extracted_color = QColor(255, 240, 200)
            elif method == "setOff":
                extracted_color = QColor(20, 20, 20)

            if extracted_color:
                extracted_qcolors.append(extracted_color)
                col_chip.setStyleSheet(
                    f"background-color: rgb({extracted_color.red()},{extracted_color.green()},{extracted_color.blue()}); "
                    f"border: 1px solid #555; border-radius: 3px;"
                )

            self.actions_table.setItem(row, 0, QTableWidgetItem(method))
            self.actions_table.setCellWidget(row, 1, col_chip)
            self.actions_table.setItem(row, 2, QTableWidgetItem(summary))

        # Update visual preview gradient bar
        self.preview_bar.set_colors(extracted_qcolors)

    def _on_name_changed(self):
        new_name = self.name_edit.text().strip()
        if not new_name or not self.current_scene_key or new_name == self.current_scene_key:
            return
        if new_name in self.data["scenes"]:
            QMessageBox.warning(self, "Error", "Scene name already exists.")
            self.name_edit.setText(self.current_scene_key)
            return

        self.data["scenes"][new_name] = self.data["scenes"].pop(self.current_scene_key)
        self.current_scene_key = new_name
        self.refresh_scene_list()

    def _on_desc_changed(self, text):
        if self.current_scene_key:
            self.data["scenes"][self.current_scene_key]["description"] = text

    def _on_loop_changed(self, checked):
        if self.current_scene_key:
            self.data["scenes"][self.current_scene_key]["loop"] = checked

    def add_scene(self):
        base_name = "new_scene"
        count = 1
        name = base_name
        while name in self.data["scenes"]:
            name = f"{base_name}_{count}"
            count += 1

        self.data["scenes"][name] = {
            "description": "New visual scene",
            "loop": False,
            "actions": [],
        }
        self.refresh_scene_list()

    def delete_scene(self):
        if not self.current_scene_key:
            return
        del self.data["scenes"][self.current_scene_key]
        self.current_scene_key = None
        self.refresh_scene_list()

    def add_action(self):
        if not self.current_scene_key:
            return
        dialog = GraphicalActionDialog(self)
        if dialog.exec() == QDialog.DialogCode.Accepted and dialog.result_data:
            self.data["scenes"][self.current_scene_key]["actions"].append(
                dialog.result_data
            )
            self.refresh_actions_table()

    def duplicate_action(self):
        selected_rows = self.actions_table.selectedItems()
        if not selected_rows or not self.current_scene_key:
            return
        row = self.actions_table.row(selected_rows[0])
        actions = self.data["scenes"][self.current_scene_key]["actions"]
        dup_action = copy.deepcopy(actions[row])
        actions.insert(row + 1, dup_action)
        self.refresh_actions_table()
        self.actions_table.selectRow(row + 1)

    def edit_action(self):
        selected_rows = self.actions_table.selectedItems()
        if not selected_rows or not self.current_scene_key:
            return
        row = self.actions_table.row(selected_rows[0])
        action = self.data["scenes"][self.current_scene_key]["actions"][row]

        dialog = GraphicalActionDialog(self, action_data=action)
        if dialog.exec() == QDialog.DialogCode.Accepted and dialog.result_data:
            self.data["scenes"][self.current_scene_key]["actions"][row] = (
                dialog.result_data
            )
            self.refresh_actions_table()

    def remove_action(self):
        selected_rows = self.actions_table.selectedItems()
        if not selected_rows or not self.current_scene_key:
            return
        row = self.actions_table.row(selected_rows[0])
        del self.data["scenes"][self.current_scene_key]["actions"][row]
        self.refresh_actions_table()

    def move_action_up(self):
        selected_rows = self.actions_table.selectedItems()
        if not selected_rows or not self.current_scene_key:
            return
        row = self.actions_table.row(selected_rows[0])
        if row <= 0:
            return
        actions = self.data["scenes"][self.current_scene_key]["actions"]
        actions[row], actions[row - 1] = actions[row - 1], actions[row]
        self.refresh_actions_table()
        self.actions_table.selectRow(row - 1)

    def move_action_down(self):
        selected_rows = self.actions_table.selectedItems()
        if not selected_rows or not self.current_scene_key:
            return
        row = self.actions_table.row(selected_rows[0])
        actions = self.data["scenes"][self.current_scene_key]["actions"]
        if row >= len(actions) - 1:
            return
        actions[row], actions[row + 1] = actions[row + 1], actions[row]
        self.refresh_actions_table()
        self.actions_table.selectRow(row + 1)

    def save_toml(self):
        path, _ = QFileDialog.getSaveFileName(
            self, "Save SceneScript File", "scenes.toml", "SceneScript TOML Files (*.toml)"
        )
        if not path:
            return
        try:
            dump_scenes_toml(self.data, path)
            QMessageBox.information(self, "Success", "SceneScript saved successfully!")
        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to save SceneScript File:\n{e}")


if __name__ == "__main__":
    app = QApplication(sys.argv)
    window = SceneScriptVisualApp()
    window.show()
    sys.exit(app.exec())